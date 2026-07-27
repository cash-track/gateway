package headers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
)

func TestHandler(t *testing.T) {
	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.Set(CfConnectingIP, "192.168.1.2")

	h := Handler(func(ctx *fasthttp.RequestCtx) {})
	h(&ctx)

	ip := ctx.UserValueBytes(clientIpUserValue).(string)
	assert.Equal(t, "192.168.1.2", ip)
	assert.Equal(t, ContentTypeJson, ctx.Response.Header.ContentType())
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
