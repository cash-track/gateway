package headers

import (
	"strings"
	"testing"

	"github.com/cash-track/gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

// spyHandler records whether/how many times the inner handler was invoked, so tests can
// assert a short-circuited preflight never reaches routing/csrf/captcha/forwarding.
func spyHandler() (fasthttp.RequestHandler, *int) {
	calls := 0

	return func(ctx *fasthttp.RequestCtx) {
		calls++
	}, &calls
}

func TestCorsHandlerPreflightAllowedOrigin(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodOptions)
	ctx.Request.Header.Set(Origin, "Test.Com")
	ctx.Request.Header.Set(AccessControlRequestMethod, fasthttp.MethodPut)
	ctx.Request.Header.Set(AccessControlRequestHeaders, "X-Custom-Header, Content-Type")
	ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")

	inner, calls := spyHandler()
	handler := CorsHandler(inner)
	handler(&ctx)

	assert.Equal(t, fasthttp.StatusNoContent, ctx.Response.StatusCode())
	assert.Equal(t, "test.com", string(ctx.Response.Header.Peek(AccessControlAllowOrigin)))
	assert.Equal(t, "true", string(ctx.Response.Header.Peek(AccessControlAllowCredentials)))
	assert.Equal(t, strings.Join(CorsAllowedMethods, ","), string(ctx.Response.Header.Peek(AccessControlAllowMethods)))
	assert.Equal(t, "X-Custom-Header, Content-Type", string(ctx.Response.Header.Peek(AccessControlAllowHeaders)))
	assert.Equal(t, "600", string(ctx.Response.Header.Peek(AccessControlMaxAge)))
	assert.Equal(t, "Origin, Access-Control-Request-Headers", string(ctx.Response.Header.Peek(Vary)))
	assert.Equal(t, 0, *calls, "inner handler must not be invoked for a true preflight request")
}

// Idempotency-Key is a non-simple header sent on every mutating request: preflight must
// reflect it back or the browser blocks the request. Allow-Headers is built from what the
// browser asked for, so no fixed allow-list names it.
func TestCorsHandlerPreflightAllowsIdempotencyKeyHeader(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodOptions)
	ctx.Request.Header.Set(Origin, "Test.Com")
	ctx.Request.Header.Set(AccessControlRequestMethod, fasthttp.MethodPost)
	ctx.Request.Header.Set(AccessControlRequestHeaders, "Idempotency-Key, Content-Type")
	ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")

	inner, calls := spyHandler()
	handler := CorsHandler(inner)
	handler(&ctx)

	assert.Equal(t, fasthttp.StatusNoContent, ctx.Response.StatusCode())
	assert.Equal(t, "Idempotency-Key, Content-Type", string(ctx.Response.Header.Peek(AccessControlAllowHeaders)))
	assert.Equal(t, 0, *calls, "inner handler must not be invoked for a true preflight request")
}

func TestCorsHandlerPreflightDisallowedOrigin(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodOptions)
	ctx.Request.Header.Set(Origin, "evil.com")
	ctx.Request.Header.Set(AccessControlRequestMethod, fasthttp.MethodPut)
	ctx.Request.Header.Set(AccessControlRequestHeaders, "X-Custom-Header")
	ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")

	inner, calls := spyHandler()
	handler := CorsHandler(inner)
	handler(&ctx)

	assert.Equal(t, fasthttp.StatusNoContent, ctx.Response.StatusCode())
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowOrigin))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowCredentials))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowMethods))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowHeaders))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlMaxAge))
	assert.Empty(t, ctx.Response.Header.Peek(Vary))
	assert.Equal(t, 0, *calls, "inner handler must not be invoked for a true preflight request")
}

func TestCorsHandlerDebugHttpDisabledSkipsLogging(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}
	orig := config.Global.DebugHttp
	config.Global.DebugHttp = false
	defer func() { config.Global.DebugHttp = orig }()

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.Set(Origin, "Test.Com")

	inner, calls := spyHandler()
	handler := CorsHandler(inner)
	handler(&ctx)

	assert.Equal(t, "test.com", string(ctx.Response.Header.Peek(AccessControlAllowOrigin)))
	assert.Equal(t, 1, *calls)
}

func TestCorsHandlerActualRequestAllowedOrigin(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.Set(Origin, "Test.Com")
	ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")

	inner, calls := spyHandler()
	handler := CorsHandler(inner)
	handler(&ctx)

	assert.Equal(t, "test.com", string(ctx.Response.Header.Peek(AccessControlAllowOrigin)))
	assert.Equal(t, "true", string(ctx.Response.Header.Peek(AccessControlAllowCredentials)))
	assert.Equal(t, "Origin", string(ctx.Response.Header.Peek(Vary)))
	assert.Equal(t, strings.Join(CorsExposedHeaders, ","), string(ctx.Response.Header.Peek(AccessControlExposeHeaders)))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowMethods))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowHeaders))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlMaxAge))
	assert.Equal(t, 1, *calls, "inner handler must be invoked for an actual request")
}

func TestCorsHandlerActualRequestDisallowedOrigin(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.Set(Origin, "a.Test.Com")
	ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")

	inner, calls := spyHandler()
	handler := CorsHandler(inner)
	handler(&ctx)

	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowOrigin))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowMethods))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowHeaders))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowCredentials))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlExposeHeaders))
	assert.Empty(t, ctx.Response.Header.Peek(Vary))
	assert.Equal(t, 1, *calls, "request must still be processed normally without an allowed origin")
}

func TestCorsHandlerActualRequestMissingOrigin(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")

	inner, calls := spyHandler()
	handler := CorsHandler(inner)
	handler(&ctx)

	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowOrigin))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowCredentials))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlExposeHeaders))
	assert.Equal(t, 1, *calls)
}

func TestCorsHandlerHealthPathIgnoresPreflight(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodOptions)
	ctx.Request.Header.Set(Origin, "Test.Com")
	ctx.Request.Header.Set(AccessControlRequestMethod, fasthttp.MethodGet)
	ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")
	ctx.Request.URI().SetPath("/live")

	inner, calls := spyHandler()
	handler := CorsHandler(inner)
	handler(&ctx)

	assert.NotEqual(t, fasthttp.StatusNoContent, ctx.Response.StatusCode())
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowOrigin))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlMaxAge))
	assert.Equal(t, 1, *calls, "health paths must fall through to the inner handler, never short-circuited")
}

func TestCorsHandlerHealthPathIgnoresActualRequest(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.Set(Origin, "a.Test.Com")
	ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")
	ctx.Request.URI().SetPath("/live")

	inner, calls := spyHandler()
	handler := CorsHandler(inner)
	handler(&ctx)

	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowOrigin))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowMethods))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowHeaders))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowCredentials))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlExposeHeaders))
	assert.Equal(t, 1, *calls)
}

// /%6Cive decodes to /live, but fasthttp/router dispatches on the raw, undecoded path
// and would 404 this request. isHealthPath must agree with the router and not treat the
// encoded lookalike as the real health probe.
func TestCorsHandlerDoesNotIgnorePercentEncodedLookalike(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.Set(Origin, "Test.Com")
	ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")
	ctx.Request.URI().SetPath("/%6Cive")

	inner, calls := spyHandler()
	handler := CorsHandler(inner)
	handler(&ctx)

	assert.Equal(t, "test.com", string(ctx.Response.Header.Peek(AccessControlAllowOrigin)))
	assert.Equal(t, 1, *calls)
}

// An OPTIONS request without Access-Control-Request-Method is not a genuine preflight —
// it must not be short-circuited and instead falls through like any other request.
func TestCorsHandlerOptionsWithoutRequestMethodIsNotPreflight(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodOptions)
	ctx.Request.Header.Set(Origin, "Test.Com")
	ctx.Request.Header.Set(XForwardedFor, "127.0.0.1")

	inner, calls := spyHandler()
	handler := CorsHandler(inner)
	handler(&ctx)

	assert.NotEqual(t, fasthttp.StatusNoContent, ctx.Response.StatusCode())
	assert.Equal(t, "test.com", string(ctx.Response.Header.Peek(AccessControlAllowOrigin)))
	assert.Equal(t, "true", string(ctx.Response.Header.Peek(AccessControlAllowCredentials)))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowMethods))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlAllowHeaders))
	assert.Empty(t, ctx.Response.Header.Peek(AccessControlMaxAge))
	assert.Equal(t, 1, *calls, "an OPTIONS request without Access-Control-Request-Method must fall through")
}
