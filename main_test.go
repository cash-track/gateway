package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/mocks"
)

// Pins the chain order: headers must wrap csrf, otherwise a CSRF-rejected request never
// reaches headers.Handler and comes back without the provenance headers.
func TestBuildHandlerCsrfRejectionStillGetsGatewayHeaders(t *testing.T) {
	original := config.Global
	t.Cleanup(func() { config.Global = original })

	config.Global.CsrfEnabled = true
	config.Global.GitTag = "v1.2.3"
	config.Global.GitSha = "abc123"
	config.Global.Compress = false
	config.Global.CorsAllowedOrigins = map[string]bool{}

	ctrl := gomock.NewController(t)
	csrf := mocks.NewCsrfHandlerMock(ctrl)
	csrf.EXPECT().Handler(gomock.Any()).DoAndReturn(func(_ fasthttp.RequestHandler) fasthttp.RequestHandler {
		// mirrors RedisHandler.Handler: writes the 417 without calling its inner handler
		return func(ctx *fasthttp.RequestCtx) {
			ctx.SetStatusCode(fasthttp.StatusExpectationFailed)
		}
	})

	innerCalled := false
	h := buildHandler(func(ctx *fasthttp.RequestCtx) {
		innerCalled = true
	}, csrf)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	h(ctx)

	assert.False(t, innerCalled, "router must never be reached once CSRF rejects the request")
	assert.Equal(t, fasthttp.StatusExpectationFailed, ctx.Response.StatusCode())
	assert.Equal(t, "v1.2.3", string(ctx.Response.Header.Peek(headers.XCtGatewayVersion)))
	assert.Equal(t, "abc123", string(ctx.Response.Header.Peek(headers.XCtGatewaySha)))
	assert.Equal(t, "max-age=31536000; includeSubDomains", string(ctx.Response.Header.Peek(headers.StrictTransportSecurity)))
	assert.Equal(t, "nosniff", string(ctx.Response.Header.Peek(headers.XContentTypeOptions)))
	assert.Equal(t, "DENY", string(ctx.Response.Header.Peek(headers.XFrameOptions)))
	assert.Equal(t, "strict-origin-when-cross-origin", string(ctx.Response.Header.Peek(headers.ReferrerPolicy)))
	assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", string(ctx.Response.Header.Peek(headers.ContentSecurityPolicy)))
}

func TestBuildHandlerCsrfDisabledStillGetsGatewayHeaders(t *testing.T) {
	original := config.Global
	t.Cleanup(func() { config.Global = original })

	config.Global.CsrfEnabled = false
	config.Global.GitTag = "v1.2.3"
	config.Global.GitSha = "abc123"
	config.Global.Compress = false
	config.Global.CorsAllowedOrigins = map[string]bool{}

	ctrl := gomock.NewController(t)
	csrf := mocks.NewCsrfHandlerMock(ctrl)
	// no EXPECT: csrf.Handler must not be called when CSRF is disabled

	innerCalled := false
	h := buildHandler(func(ctx *fasthttp.RequestCtx) {
		innerCalled = true
		ctx.SetStatusCode(fasthttp.StatusOK)
	}, csrf)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	h(ctx)

	assert.True(t, innerCalled)
	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Equal(t, "v1.2.3", string(ctx.Response.Header.Peek(headers.XCtGatewayVersion)))
	assert.Equal(t, "abc123", string(ctx.Response.Header.Peek(headers.XCtGatewaySha)))
	assert.Equal(t, "max-age=31536000; includeSubDomains", string(ctx.Response.Header.Peek(headers.StrictTransportSecurity)))
	assert.Equal(t, "nosniff", string(ctx.Response.Header.Peek(headers.XContentTypeOptions)))
	assert.Equal(t, "DENY", string(ctx.Response.Header.Peek(headers.XFrameOptions)))
	assert.Equal(t, "strict-origin-when-cross-origin", string(ctx.Response.Header.Peek(headers.ReferrerPolicy)))
	assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", string(ctx.Response.Header.Peek(headers.ContentSecurityPolicy)))
}
