package api

import (
	"bytes"
	"fmt"
	"log"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

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

	if err := s.http.Do(req, resp); err != nil {
		span.RecordError(err)
		return fmt.Errorf("API request error: %w", err)
	}

	logger.DebugResponse(resp, ServiceId)
	logger.FullForwarded(ctx, req, resp, ServiceId)

	span.SetAttributes(traces.ResponseAttributes(resp)...)

	if !auth.IsLogged() || !auth.CanRefresh() || resp.StatusCode() != fasthttp.StatusUnauthorized {
		return forwardResponse(ctx, resp)
	}

	// perform refresh token
	newAuth, err := s.refreshToken(auth, spanCtx, ctx)
	if err != nil {
		span.RecordError(err)
		log.Printf("[%s] refresh token attempt: %s", remoteIp, err.Error())
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
				)...,
			),
		)
		defer retrySpan.End()

		// execute request 2nd attempt
		if err := s.http.Do(req, resp); err != nil {
			retrySpan.RecordError(err)
			return fmt.Errorf("API request with fresh token error: %w", err)
		}

		logger.DebugResponse(resp, ServiceId)
		retrySpan.SetAttributes(traces.ResponseAttributes(resp)...)
	}

	newAuth.WriteCookie(ctx)

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
		headers.XRateLimit,
		headers.XRateLimitRemaining,
	})

	if val := ctx.Response.Header.Peek(headers.AccessControlAllowOrigin); val != nil {
		ctx.Response.Header.Set(headers.AccessControlAllowCredentials, "true")
	}

	return nil
}
