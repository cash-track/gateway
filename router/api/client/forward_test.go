package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/headers"
)

func TestForwardResponse(t *testing.T) {
	ctx := fasthttp.RequestCtx{}

	resp := fasthttp.Response{}
	resp.SetStatusCode(fasthttp.StatusUnauthorized)
	resp.SetBody([]byte("not allowed\n\r"))
	resp.Header.Set(headers.AccessControlAllowOrigin, "test.com")
	resp.Header.Set(headers.AccessControlMaxAge, "3600")
	resp.Header.Set(headers.AccessControlAllowMethods, "GET,POST")
	resp.Header.Set(headers.AccessControlAllowHeaders, "Content-Type,Accept-Language")
	resp.Header.Set(headers.ContentType, "text/plain")
	resp.Header.Set(headers.RetryAfter, "200")
	resp.Header.Set(headers.Vary, "Content-Type,X-Rate-Limit")
	resp.Header.Set(headers.XRateLimit, "123")
	resp.Header.Set(headers.XRateLimitRemaining, "2")

	err := ForwardResponse(&ctx, &resp)

	assert.NoError(t, err)

	assert.Equal(t, "test.com", string(ctx.Response.Header.Peek(headers.AccessControlAllowOrigin)))
	assert.Equal(t, "true", string(ctx.Response.Header.Peek(headers.AccessControlAllowCredentials)))
	assert.Equal(t, "GET,POST", string(ctx.Response.Header.Peek(headers.AccessControlAllowMethods)))
	assert.Equal(t, "Content-Type,Accept-Language", string(ctx.Response.Header.Peek(headers.AccessControlAllowHeaders)))
	assert.Equal(t, "3600", string(ctx.Response.Header.Peek(headers.AccessControlMaxAge)))
	assert.Equal(t, "text/plain", string(ctx.Response.Header.Peek(headers.ContentType)))
	assert.Equal(t, "200", string(ctx.Response.Header.Peek(headers.RetryAfter)))
	assert.Equal(t, "Content-Type,X-Rate-Limit", string(ctx.Response.Header.Peek(headers.Vary)))
	assert.Equal(t, "123", string(ctx.Response.Header.Peek(headers.XRateLimit)))
	assert.Equal(t, "2", string(ctx.Response.Header.Peek(headers.XRateLimitRemaining)))
}
