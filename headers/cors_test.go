package headers

import (
	"strings"
	"testing"

	"github.com/cash-track/gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestCorsHandler(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}
	config.Global.DebugHttp = true

	t.Run("Allow", func(t *testing.T) {
		ctx := fasthttp.RequestCtx{}
		ctx.Request.Header.Set(Origin, "Test.Com")
		ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")

		handler := CorsHandler(func(ctx *fasthttp.RequestCtx) {})
		handler(&ctx)

		assert.Equal(t, "true", string(ctx.Response.Header.Peek(AccessControlAllowCredentials)))
		assert.Equal(t, strings.Join(CorsAllowedMethods, ","), string(ctx.Response.Header.Peek(AccessControlAllowMethods)))
		assert.Equal(t, strings.Join(CorsAllowedHeaders, ","), string(ctx.Response.Header.Peek(AccessControlAllowHeaders)))
		assert.Equal(t, "test.com", string(ctx.Response.Header.Peek(AccessControlAllowOrigin)))
	})

	t.Run("AllowOptionsStatusAlwaysOk", func(t *testing.T) {
		ctx := fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(fasthttp.MethodOptions)
		ctx.Request.Header.Set(Origin, "Test.Com")
		ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")

		handler := CorsHandler(func(ctx *fasthttp.RequestCtx) {})
		handler(&ctx)

		assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	})

	t.Run("RejectIgnorePath", func(t *testing.T) {
		ctx := fasthttp.RequestCtx{}
		ctx.Request.Header.Set(Origin, "a.Test.Com")
		ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")
		ctx.Request.URI().SetPath("/live")

		handler := CorsHandler(func(ctx *fasthttp.RequestCtx) {})
		handler(&ctx)

		assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowOrigin))
		assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowMethods))
		assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowHeaders))
		assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowCredentials))
	})

	t.Run("RejectNotAllowed", func(t *testing.T) {
		ctx := fasthttp.RequestCtx{}
		ctx.Request.Header.Set(Origin, "a.Test.Com")
		ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")

		handler := CorsHandler(func(ctx *fasthttp.RequestCtx) {})
		handler(&ctx)

		assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowOrigin))
		assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowMethods))
		assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowHeaders))
		assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowCredentials))
	})

	t.Run("RejectAllowedByUpstream", func(t *testing.T) {
		ctx := fasthttp.RequestCtx{}
		ctx.Request.Header.Set(Origin, "Test.Com")
		ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")
		ctx.Response.Header.Set(AccessControlAllowOrigin, "test.com")

		handler := CorsHandler(func(ctx *fasthttp.RequestCtx) {})
		handler(&ctx)

		assert.Equal(t, "test.com", string(ctx.Response.Header.Peek(AccessControlAllowOrigin)))
		assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowMethods))
		assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowHeaders))
		assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowCredentials))
	})

}
