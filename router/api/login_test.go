package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/mocks"
)

func TestLogin(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{
		WebAppUrl: "https://home.com",
	}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.SetBodyString(`{"accessToken":"new_access_token","refreshToken":"new_refresh_token"}`)

	err := h.Login(&ctx)

	assert.NoError(t, err)
	assert.Equal(t, `{"redirectUrl":"https://home.com"}`, string(ctx.Response.Body()))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), "new_access_token")
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), "new_refresh_token")
}

func TestLoginBadStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{
		WebAppUrl: "https://home.com",
	}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Response.SetStatusCode(fasthttp.StatusUnauthorized)
	ctx.Response.SetBodyString(`{"accessToken":"new_access_token","refreshToken":"new_refresh_token"}`)

	err := h.Login(&ctx)

	assert.NoError(t, err)
	assert.NotContains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), "new_access_token")
	assert.NotContains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), "new_refresh_token")
}

func TestLoginInvalidResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{
		WebAppUrl: "https://home.com",
	}, s, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.SetBodyString(`{"accessToken":"new_access_token","refreshToken":"new_refresh_token`)

	err := h.Login(&ctx)

	assert.Error(t, err)
	assert.NotContains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), "new_access_token")
	assert.NotContains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), "new_refresh_token")
}
