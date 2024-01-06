package api

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/mocks"
)

func TestAuthSetHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	c.EXPECT().Verify(gomock.Any()).Return(true, nil)
	s.EXPECT().ForwardRequest(gomock.Any(), nil).DoAndReturn(func(ctx *fasthttp.RequestCtx, body []byte) error {
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.SetBodyString(`{"accessToken":"new_access_token"}`)
		return nil
	})

	h.AuthSetHandler(&ctx)

	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), "new_access_token")
}

func TestAuthSetHandlerCaptchaFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	c.EXPECT().Verify(gomock.Any()).Return(false, nil)

	h.AuthSetHandler(&ctx)

	assert.Equal(t, fasthttp.StatusBadRequest, ctx.Response.StatusCode())
}

func TestAuthSetHandlerCaptchaError(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	c.EXPECT().Verify(gomock.Any()).Return(false, fmt.Errorf("captcha api down"))

	h.AuthSetHandler(&ctx)

	assert.Equal(t, fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
}

func TestAuthSetHandlerLoginError(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	c.EXPECT().Verify(gomock.Any()).Return(true, nil)
	s.EXPECT().ForwardRequest(gomock.Any(), nil).DoAndReturn(func(ctx *fasthttp.RequestCtx, body []byte) error {
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.SetBodyString(`{"accessToken":"new_access_token"`)
		return nil
	})

	h.AuthSetHandler(&ctx)

	assert.Equal(t, fasthttp.StatusBadGateway, ctx.Response.StatusCode())
}

func TestAuthResetHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.SetCookie(cookie.RefreshTokenCookieName, "refresh_token_test")

	body := []byte(`{"refreshToken":"refresh_token_test"}`)

	s.EXPECT().ForwardRequest(gomock.Any(), body).Return(nil)

	h.AuthResetHandler(&ctx)

	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), fmt.Sprintf("%s=;", cookie.AccessTokenCookieName))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), fmt.Sprintf("%s=;", cookie.RefreshTokenCookieName))
}

func TestFullForwardedHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	s.EXPECT().ForwardRequest(gomock.Any(), nil).DoAndReturn(func(ctx *fasthttp.RequestCtx, body []byte) error {
		assert.Equal(t, fasthttp.MethodPost, string(ctx.Request.Header.Method()))
		assert.Equal(t, "Value", string(ctx.Request.Header.Peek("Test")))
		return nil
	})

	h.FullForwardedHandler(&ctx)
}

func TestFullForwardedHandlerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	s.EXPECT().ForwardRequest(gomock.Any(), nil).Return(fmt.Errorf("broken pipe"))

	h.FullForwardedHandler(&ctx)

	assert.Equal(t, fasthttp.StatusBadGateway, ctx.Response.StatusCode())
}

func TestFullForwardedHandlerRestrictedMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodConnect)
	ctx.Request.Header.Set("Test", "Value")

	h.FullForwardedHandler(&ctx)

	assert.Equal(t, fasthttp.StatusBadRequest, ctx.Response.StatusCode())
}

func TestFullForwardedHandlerWithBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	body := cookie.Auth{
		AccessToken: "123",
	}
	bodyJson := []byte(`{"accessToken":"123"}`)

	s.EXPECT().ForwardRequest(gomock.Any(), bodyJson).DoAndReturn(func(ctx *fasthttp.RequestCtx, body []byte) error {
		assert.Equal(t, fasthttp.MethodPost, string(ctx.Request.Header.Method()))
		assert.Equal(t, "Value", string(ctx.Request.Header.Peek("Test")))
		return nil
	})

	h.FullForwardedHandlerWithBody(&ctx, body)
}

func TestFullForwardedHandlerWithBodyError(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	s.EXPECT().ForwardRequest(gomock.Any(), []byte(`{"test":"123"}`)).Return(fmt.Errorf("broken pipe"))

	h.FullForwardedHandlerWithBody(&ctx, map[string]string{"test": "123"})

	assert.Equal(t, fasthttp.StatusBadGateway, ctx.Response.StatusCode())
}

func TestFullForwardedHandlerWithBodyJsonError(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	var i complex128
	h.FullForwardedHandlerWithBody(&ctx, i)

	assert.Equal(t, fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
}

func TestFullForwardedHandlerWithBodyRestrictedMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodConnect)
	ctx.Request.Header.Set("Test", "Value")

	h.FullForwardedHandlerWithBody(&ctx, nil)

	assert.Equal(t, fasthttp.StatusBadRequest, ctx.Response.StatusCode())
}

func TestHealthcheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c)

	s.EXPECT().Healthcheck().Return(nil)

	err := h.Healthcheck()

	assert.NoError(t, err)
}
