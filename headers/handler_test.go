package headers

import (
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
)

func TestHandler(t *testing.T) {
	original := config.Global.TrustedProxies
	t.Cleanup(func() { config.Global.TrustedProxies = original })
	config.Global.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	ctx.Request.Header.Set(CfConnectingIP, "192.168.1.2")

	h := Handler(func(ctx *fasthttp.RequestCtx) {})
	h(&ctx)

	ip := ctx.UserValueBytes(clientIpUserValue).(string)
	assert.Equal(t, "192.168.1.2", ip)
	assert.Equal(t, ContentTypeJson, ctx.Response.Header.ContentType())
}

func TestHandlerUntrustedPeerIgnoresHeader(t *testing.T) {
	original := config.Global.TrustedProxies
	t.Cleanup(func() { config.Global.TrustedProxies = original })
	config.Global.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("203.0.113.10")})
	ctx.Request.Header.Set(CfConnectingIP, "192.168.1.2")

	h := Handler(func(ctx *fasthttp.RequestCtx) {})
	h(&ctx)

	ip := ctx.UserValueBytes(clientIpUserValue).(string)
	assert.Equal(t, "203.0.113.10", ip)
}

func TestHandlerAddTraceId(t *testing.T) {
	id := "traceId123456"
	ctx := fasthttp.RequestCtx{}
	ctx.SetUserValue("traceIdCtx", id)

	h := Handler(func(ctx *fasthttp.RequestCtx) {})
	h(&ctx)

	traceId := string(ctx.Response.Header.Peek(XCtTraceId))
	assert.Equal(t, traceId, id)
}

func TestHandlerAddGatewayVersionHeaders(t *testing.T) {
	origTag, origSha := config.Global.GitTag, config.Global.GitSha
	t.Cleanup(func() {
		config.Global.GitTag = origTag
		config.Global.GitSha = origSha
	})
	config.Global.GitTag = "v1.2.3"
	config.Global.GitSha = "abc123"

	ctx := fasthttp.RequestCtx{}

	h := Handler(func(ctx *fasthttp.RequestCtx) {})
	h(&ctx)

	assert.Equal(t, "v1.2.3", string(ctx.Response.Header.Peek(XCtGatewayVersion)))
	assert.Equal(t, "abc123", string(ctx.Response.Header.Peek(XCtGatewaySha)))
}

func TestHandlerOmitsGatewayVersionHeadersWhenEmpty(t *testing.T) {
	origTag, origSha := config.Global.GitTag, config.Global.GitSha
	t.Cleanup(func() {
		config.Global.GitTag = origTag
		config.Global.GitSha = origSha
	})
	config.Global.GitTag = ""
	config.Global.GitSha = ""

	ctx := fasthttp.RequestCtx{}

	h := Handler(func(ctx *fasthttp.RequestCtx) {})
	h(&ctx)

	assert.Empty(t, ctx.Response.Header.Peek(XCtGatewayVersion))
	assert.Empty(t, ctx.Response.Header.Peek(XCtGatewaySha))
}

func assertSecurityHeaders(t *testing.T, ctx *fasthttp.RequestCtx) {
	t.Helper()
	assert.Equal(t, "max-age=31536000; includeSubDomains", string(ctx.Response.Header.Peek(StrictTransportSecurity)))
	assert.Equal(t, "nosniff", string(ctx.Response.Header.Peek(XContentTypeOptions)))
	assert.Equal(t, "DENY", string(ctx.Response.Header.Peek(XFrameOptions)))
	assert.Equal(t, "strict-origin-when-cross-origin", string(ctx.Response.Header.Peek(ReferrerPolicy)))
	assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", string(ctx.Response.Header.Peek(ContentSecurityPolicy)))
}

func TestHandlerAddSecurityHeaders(t *testing.T) {
	ctx := fasthttp.RequestCtx{}

	h := Handler(func(ctx *fasthttp.RequestCtx) {})
	h(&ctx)

	assertSecurityHeaders(t, &ctx)
}

// Security headers must be present even when the gateway short-circuits with an error
// (CSRF rejection, captcha rejection) and never reaches the API.
func TestHandlerAddSecurityHeadersOnErrorResponse(t *testing.T) {
	ctx := fasthttp.RequestCtx{}

	h := Handler(func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusExpectationFailed)
		ctx.SetBodyString(`{"error":"csrf"}`)
	})
	h(&ctx)

	assert.Equal(t, fasthttp.StatusExpectationFailed, ctx.Response.StatusCode())
	assertSecurityHeaders(t, &ctx)
}

func TestHandlerOmitsSecurityHeadersOnHealthPaths(t *testing.T) {
	for path := range healthPaths {
		t.Run(path, func(t *testing.T) {
			ctx := fasthttp.RequestCtx{}
			ctx.Request.URI().SetPath(path)

			h := Handler(func(ctx *fasthttp.RequestCtx) {})
			h(&ctx)

			assert.Empty(t, ctx.Response.Header.Peek(StrictTransportSecurity))
			assert.Empty(t, ctx.Response.Header.Peek(XContentTypeOptions))
			assert.Empty(t, ctx.Response.Header.Peek(XFrameOptions))
			assert.Empty(t, ctx.Response.Header.Peek(ReferrerPolicy))
			assert.Empty(t, ctx.Response.Header.Peek(ContentSecurityPolicy))
		})
	}
}

// /%6Cive decodes to /live, but fasthttp/router dispatches on the raw, undecoded
// path and would 404 this request. Security headers must still be written, since
// isHealthPath matches the router's raw path, not the decoded one.
func TestHandlerAddSecurityHeadersOnPercentEncodedHealthPathLookalike(t *testing.T) {
	ctx := fasthttp.RequestCtx{}
	ctx.Request.URI().SetPath("/%6Cive")

	h := Handler(func(ctx *fasthttp.RequestCtx) {})
	h(&ctx)

	assertSecurityHeaders(t, &ctx)
}

// CorsHandler calls its inner handler unconditionally before writing CORS headers, so a
// preflight OPTIONS request still passes through headers.Handler and gets security headers.
func TestSecurityHeadersPresentOnCorsPreflight(t *testing.T) {
	config.Global.CorsAllowedOrigins = map[string]bool{"test.com": true}

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodOptions)
	ctx.Request.Header.Set(Origin, "test.com")

	h := CorsHandler(Handler(func(ctx *fasthttp.RequestCtx) {}))
	h(&ctx)

	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Equal(t, "test.com", string(ctx.Response.Header.Peek(AccessControlAllowOrigin)))
	assertSecurityHeaders(t, &ctx)
}
