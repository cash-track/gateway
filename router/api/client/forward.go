package client

import (
	"bytes"
	"fmt"
	"log"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/logger"
)

func ForwardRequest(ctx *fasthttp.RequestCtx, body []byte) error {
	// prepare req based on incoming ctx.Request
	req := fasthttp.AcquireRequest()
	defer func() {
		fasthttp.ReleaseRequest(req)
	}()

	remoteIp := headers.GetClientIPFromContext(ctx)

	req.Header.SetMethodBytes(bytes.Clone(ctx.Request.Header.Method()))
	req.SetRequestURI(prepareRequestURI(ctx.Request.URI().Path(), ctx.Request.URI().QueryString()))

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

	logger.DebugRequest(req, Service)

	// execute request
	resp := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseResponse(resp)
	}()

	if err := client.Do(req, resp); err != nil {
		return fmt.Errorf("API request error: %w", err)
	}

	logger.DebugResponse(resp, Service)
	logger.FullForwarded(ctx, req, resp, Service)

	if !auth.IsLogged() || !auth.CanRefresh() || resp.StatusCode() != fasthttp.StatusUnauthorized {
		return ForwardResponse(ctx, resp)
	}

	// perform refresh token
	newAuth, err := refreshToken(auth)
	if err != nil {
		log.Printf("[%s] refresh token attempt: %s", remoteIp, err.Error())
	}

	if newAuth.IsLogged() {
		headers.WriteBearerToken(req, newAuth.AccessToken)

		// execute request 2nd attempt
		if err := client.Do(req, resp); err != nil {
			return fmt.Errorf("API request with fresh token error: %w", err)
		}

		logger.DebugResponse(resp, Service)
	}

	newAuth.WriteCookie(ctx)

	return ForwardResponse(ctx, resp)
}

func ForwardResponse(ctx *fasthttp.RequestCtx, resp *fasthttp.Response) error {
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
