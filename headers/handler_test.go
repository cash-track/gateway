package headers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
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
