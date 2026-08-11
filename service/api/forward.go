package api

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/logger"
	"github.com/cash-track/gateway/traces"
)

func (s *HttpService) ForwardRequest(ctx *fasthttp.RequestCtx, body []byte) error {
	// prepare req based on incoming ctx.Request
	req := fasthttp.AcquireRequest()
	defer func() {
		fasthttp.ReleaseRequest(req)
	}()

	remoteIp := headers.GetClientIPFromContext(ctx)

	req.Header.SetMethodBytes(bytes.Clone(ctx.Request.Header.Method()))
	s.copyRequestURI(ctx.Request.URI(), req.URI())

	req.Header.SetContentTypeBytes(headers.ContentTypeJson)
	req.Header.SetBytesV(headers.Accept, headers.ContentTypeJson)
	req.Header.Set(headers.XForwardedFor, remoteIp)

	// set once: the refreshed-token retry below reuses this same req
	headers.WriteGatewayVersion(&req.Header, s.config.GitTag, s.config.GitSha)
	headers.WriteGatewaySecret(&req.Header, s.config.GatewaySecret)

	headers.CopyFromRequest(ctx, req, []string{
		headers.AcceptLanguage,
		headers.AccessControlRequestHeaders,
		headers.AccessControlRequestMethod,
		headers.ContentType,
		headers.UserAgent,
		headers.Referer,
		headers.Origin,
	})

	headers.CopyCloudFlareHeaders(ctx, req)

	// propagate authentication
	auth := cookie.ReadAuthCookie(ctx)
	if auth.IsLogged() {
		headers.WriteBearerToken(req, auth.AccessToken)
	}

	// copy Body if method allows
	if _, ok := methodsWithBody[string(ctx.Method())]; ok {
		if body == nil {
			req.SetBody(bytes.Clone(ctx.Request.Body()))
		} else {
			req.SetBody(bytes.Clone(body))
		}
	}

	logger.DebugRequest(req, ServiceId)

	spanCtx, span := traces.GetTracer().Start(
		traces.FindParentContext(ctx),
		fmt.Sprintf("forward %s %s %s", ServiceId, ctx.Request.Header.Method(), ctx.URI().PathOriginal()),
		trace.WithAttributes(
			traces.MergeAttributes(
				traces.Attributes(attribute.String("http.request.real_ip", remoteIp)),
				traces.AttributesGetter(auth),
				traces.RequestAttributes(req),
				requestBodyAttributes(req),
			)...,
		),
	)
	defer span.End()

	traces.PropagateContextToRequest(ctx, req)

	// execute request
	resp := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseResponse(resp)
	}()

	start := time.Now()
	err := s.doWithBreaker(req, resp)
	duration := time.Since(start)

	if err != nil {
		span.RecordError(err)

		return fmt.Errorf("API request error: %w", err)
	}

	logger.DebugResponse(resp, ServiceId)
	logger.FullForwarded(ctx, resp, ServiceId, duration)

	span.SetAttributes(traces.ResponseAttributes(resp)...)
	span.SetAttributes(responseBodyAttributes(resp)...)

	if !auth.IsLogged() || !auth.CanRefresh() || resp.StatusCode() != fasthttp.StatusUnauthorized {
		return forwardResponse(ctx, resp)
	}

	// perform refresh token
	newAuth, err := s.refreshToken(auth, spanCtx, ctx)
	if err != nil {
		// Transient failure: could not reach the API or it returned a non-401
		// (e.g. 5xx). The refresh token may still be valid, so DO NOT delete
		// cookies / log the user out. Preserve the session and return a
		// retryable status; the client can retry once the API recovers.
		span.RecordError(err)
		slog.Warn("refresh token attempt failed, keeping session (transient)",
			"trace_id", traces.FindTraceId(ctx),
			"client_ip", remoteIp,
			"error", err,
		)

		resp.Reset()
		resp.SetStatusCode(fasthttp.StatusServiceUnavailable)

		// WriteCookie deliberately NOT called → cookies untouched.
		return forwardResponse(ctx, resp)
	}

	if newAuth.IsLogged() {
		headers.WriteBearerToken(req, newAuth.AccessToken)

		span.End()

		_, retrySpan := traces.GetTracer().Start(
			spanCtx,
			fmt.Sprintf("forward (refreshed) %s %s %s", ServiceId, ctx.Request.Header.Method(), ctx.URI().PathOriginal()),
			trace.WithAttributes(
				traces.MergeAttributes(
					traces.Attributes(attribute.String("http.request.real_ip", remoteIp)),
					traces.AttributesGetter(auth),
					traces.RequestAttributes(req),
					requestBodyAttributes(req),
				)...,
			),
		)
		defer retrySpan.End()

		// execute request 2nd attempt
		retryStart := time.Now()
		retryErr := s.doWithBreaker(req, resp)
		retryDuration := time.Since(retryStart)

		if retryErr != nil {
			retrySpan.RecordError(retryErr)

			return fmt.Errorf("API request with fresh token error: %w", retryErr)
		}

		logger.DebugResponse(resp, ServiceId)
		// Logged separately from the initial attempt above: this is a second, genuinely
		// distinct round trip to the API (with a refreshed token), matching the separate
		// "forward (refreshed)" trace span created for it.
		logger.FullForwarded(ctx, resp, ServiceId, retryDuration)
		retrySpan.SetAttributes(traces.ResponseAttributes(resp)...)
		retrySpan.SetAttributes(responseBodyAttributes(resp)...)

		// Seed a fresh CSRF token keyed to the new access token's iat so the
		// next mutating request is not rejected with 417. Non-fatal: the user
		// can recover via GET /csrf if Redis is temporarily unavailable.
		// Only seed on 2xx to avoid advancing CSRF state when the retried request fails.
		if s.csrf != nil && resp.StatusCode() >= fasthttp.StatusOK && resp.StatusCode() < fasthttp.StatusMultipleChoices {
			if err := s.csrf.Seed(ctx, newAuth); err != nil {
				slog.Warn("csrf seed after token refresh failed",
					"trace_id", traces.FindTraceId(ctx),
					"client_ip", remoteIp,
					"error", err,
				)
			}
		}
	}

	// This is also reached when newAuth is not logged (refresh token genuinely
	// expired/invalid) ⇒ WriteCookie takes the delete branch, logging the user out.
	if err := newAuth.WriteCookie(ctx); err != nil {
		span.RecordError(err)

		return fmt.Errorf("write auth cookie after refresh: %w", err)
	}

	return forwardResponse(ctx, resp)
}

func forwardResponse(ctx *fasthttp.RequestCtx, resp *fasthttp.Response) error {
	ctx.SetStatusCode(resp.StatusCode())
	ctx.SetBody(bytes.Clone(resp.Body()))

	headers.CopyFromResponse(resp, ctx, []string{
		headers.AccessControlAllowOrigin,
		headers.AccessControlAllowMethods,
		headers.AccessControlAllowHeaders,
		headers.AccessControlMaxAge,
		headers.ContentType,
		headers.RetryAfter,
		headers.Vary,
		headers.XCtApiSha,
		headers.XCtApiVersion,
		headers.XRateLimit,
		headers.XRateLimitRemaining,
	})

	if val := ctx.Response.Header.Peek(headers.AccessControlAllowOrigin); val != nil {
		ctx.Response.Header.Set(headers.AccessControlAllowCredentials, "true")
		ctx.Response.Header.Set(headers.AccessControlExposeHeaders, strings.Join(headers.CorsExposedHeaders, ","))
	}

	return nil
}

// requestBodyAttributes returns a redacted body span attribute for req, or nil when
// body capture is disabled via TRACE_CAPTURE_BODY.
func requestBodyAttributes(req *fasthttp.Request) []attribute.KeyValue {
	if !config.Global.TraceCaptureBody {
		return nil
	}

	return traces.Attributes(traces.RequestBodyAttribute(req))
}

// responseBodyAttributes returns a redacted body span attribute for resp, or nil when
// body capture is disabled via TRACE_CAPTURE_BODY.
func responseBodyAttributes(resp *fasthttp.Response) []attribute.KeyValue {
	if !config.Global.TraceCaptureBody {
		return nil
	}

	return traces.Attributes(traces.ResponseBodyAttribute(resp))
}
