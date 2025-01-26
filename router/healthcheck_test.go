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
	a := mocks.NewApiHandlerMock(ctrl)
	c := mocks.NewCsrfHandlerMock(ctrl)
	r := New(a, c)

	ctx := fasthttp.RequestCtx{}

	r.LiveHandler(&ctx)

	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Equal(t, "ok", string(ctx.Response.Body()))
}

func TestReadyHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := mocks.NewApiHandlerMock(ctrl)
	a.EXPECT().Healthcheck().Return(nil)
	c := mocks.NewCsrfHandlerMock(ctrl)
	r := New(a, c)

	ctx := fasthttp.RequestCtx{}

	r.ReadyHandler(&ctx)

	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Equal(t, "ok", string(ctx.Response.Body()))
}

func TestReadyHandlerFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := mocks.NewApiHandlerMock(ctrl)
	a.EXPECT().Healthcheck().Return(fmt.Errorf("context cancelled"))
	c := mocks.NewCsrfHandlerMock(ctrl)
	r := New(a, c)

	ctx := fasthttp.RequestCtx{}

	r.ReadyHandler(&ctx)

	assert.Equal(t, fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
	assert.Equal(t, "[api] nok", string(ctx.Response.Body()))
}
