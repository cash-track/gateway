package router

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/mocks"
)

func TestLiveHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewApiHandlerMock(ctrl)
	r := New(h)

	ctx := fasthttp.RequestCtx{}

	r.LiveHandler(&ctx)

	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Equal(t, "ok", string(ctx.Response.Body()))
}

func TestReadyHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewApiHandlerMock(ctrl)
	h.EXPECT().Healthcheck().Return(nil)
	r := New(h)

	ctx := fasthttp.RequestCtx{}

	r.ReadyHandler(&ctx)

	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Equal(t, "ok", string(ctx.Response.Body()))
}

func TestReadyHandlerFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewApiHandlerMock(ctrl)
	h.EXPECT().Healthcheck().Return(fmt.Errorf("context cancelled"))
	r := New(h)

	ctx := fasthttp.RequestCtx{}

	r.ReadyHandler(&ctx)

	assert.Equal(t, fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
	assert.Equal(t, "[api] nok", string(ctx.Response.Body()))
}
