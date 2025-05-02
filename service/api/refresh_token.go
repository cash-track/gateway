package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/logger"
	"github.com/cash-track/gateway/traces"
)

var refreshURI = []byte("/auth/refresh")

func (s *HttpService) refreshToken(auth cookie.Auth, spanCtx context.Context, ctx *fasthttp.RequestCtx) (cookie.Auth, error) {
	req := fasthttp.AcquireRequest()
	defer func() {
		fasthttp.ReleaseRequest(req)
	}()

	req.Header.SetMethod(fasthttp.MethodPost)
	s.setRequestURI(req.URI(), refreshURI)
	req.Header.SetContentTypeBytes(headers.ContentTypeJson)
	req.Header.SetBytesV(headers.Accept, headers.ContentTypeJson)
	headers.WriteBearerToken(req, auth.RefreshToken)

	data, _ := json.Marshal(cookie.Auth{AccessToken: auth.AccessToken})
	req.SetBody(data)

	logger.DebugRequest(req, ServiceId)
	traces.PropagateContextToRequest(ctx, req)

	// execute request
	resp := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseResponse(resp)
	}()

	_, span := traces.GetTracer().Start(
		spanCtx,
		fmt.Sprintf("refresh token %s %s %s", ServiceId, req.Header.Method(), req.URI().PathOriginal()),
		trace.WithAttributes(
			traces.MergeAttributes(
				traces.AttributesGetter(auth),
				traces.RequestAttributes(req),
			)...,
		),
	)
	defer span.End()

	newAuth := cookie.Auth{}
	err := s.http.Do(req, resp)
	if err != nil {
		span.RecordError(err)
		return newAuth, fmt.Errorf("refresh token API request error: %w", err)
	}

	logger.DebugResponse(resp, ServiceId)
	span.SetAttributes(traces.ResponseAttributes(resp)...)

	if resp.StatusCode() == fasthttp.StatusUnauthorized {
		// re-login required
		span.SetStatus(codes.Error, "unauthorized")
		return newAuth, nil
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		// unexpected status
		err = fmt.Errorf("refresh token failed [status %d]: %v", resp.StatusCode(), resp.Body())
		span.SetStatus(codes.Error, "unknown")
		span.RecordError(err)
		return newAuth, err
	}

	if err := json.Unmarshal(resp.Body(), &newAuth); err != nil {
		span.RecordError(err)
		return newAuth, fmt.Errorf("refresh token unexpected response body: %w", err)
	}

	return newAuth, nil
}
