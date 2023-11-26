package headers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestCopyFromRequest(t *testing.T) {
	ctx := fasthttp.RequestCtx{}
	req := fasthttp.Request{}

	ctx.Request.Header.Set(AcceptLanguage, "uk,en")
	ctx.Request.Header.Set(Origin, "test.com")
	ctx.Request.Header.Set(Referer, "google.com")
	ctx.Request.Header.Set(XForwardedFor, "127.0.0.1, 192.168.1.1")

	req.Header.Set(XForwardedFor, "10.0.0.1")

	CopyFromRequest(&ctx, &req, []string{AcceptLanguage, Origin, XForwardedFor})

	assert.Equal(t, "uk,en", string(req.Header.Peek(AcceptLanguage)))
	assert.Equal(t, "test.com", string(req.Header.Peek(Origin)))
	assert.Equal(t, "127.0.0.1, 192.168.1.1, 10.0.0.1", string(req.Header.Peek(XForwardedFor)))
	assert.Empty(t, req.Header.Peek(Referer))
}

func TestCopyFromResponse(t *testing.T) {
	resp := fasthttp.Response{}
	ctx := fasthttp.RequestCtx{}

	resp.Header.Set(AccessControlAllowOrigin, "test.com")
	resp.Header.Set(XRateLimit, "12,32")
	resp.Header.Set(ContentType, "text/html")
	resp.Header.Set(XRealIp, "127.0.0.1")
	resp.Header.Set(XForwardedFor, "192.168.1.1")

	ctx.Response.Header.Set(ContentType, "application/json")
	ctx.Response.Header.Set(XRealIp, "10.0.0.1")

	CopyFromResponse(&resp, &ctx, []string{AccessControlAllowOrigin, XRateLimit, ContentType, XRealIp})

	assert.Equal(t, "test.com", string(ctx.Response.Header.Peek(AccessControlAllowOrigin)))
	assert.Equal(t, "12,32", string(ctx.Response.Header.Peek(XRateLimit)))
	assert.Equal(t, "text/html", string(ctx.Response.Header.Peek(ContentType)))
	assert.Equal(t, "127.0.0.1, 10.0.0.1", string(ctx.Response.Header.Peek(XRealIp)))
	assert.Empty(t, ctx.Response.Header.Peek(XForwardedFor))
}
