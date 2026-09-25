package router

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
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

func TestReadyHandlerLogsOnlyStateChanges(t *testing.T) {
	var out bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctrl := gomock.NewController(t)
	a := mocks.NewApiHandlerMock(ctrl)
	gomock.InOrder(
		a.EXPECT().Healthcheck().Return(fmt.Errorf("down")).Times(2),
		a.EXPECT().Healthcheck().Return(nil).Times(2),
	)
	r := New(a, mocks.NewCsrfHandlerMock(ctrl))

	for range 4 {
		r.ReadyHandler(&fasthttp.RequestCtx{})
	}

	assert.Equal(t, 1, strings.Count(out.String(), "API not ready"))
	assert.Equal(t, 1, strings.Count(out.String(), "API ready again"))
}
