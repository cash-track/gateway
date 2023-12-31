package healthcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestLiveHandler(t *testing.T) {
	ctx := fasthttp.RequestCtx{}

	LiveHandler(&ctx)

	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Equal(t, "ok", string(ctx.Response.Body()))
}
