package router

import (
	"testing"

	"github.com/cash-track/gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestLiveHandler(t *testing.T) {
	ctx := fasthttp.RequestCtx{}
	r := New(config.Config{})

	r.LiveHandler(&ctx)

	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Equal(t, "ok", string(ctx.Response.Body()))
}
