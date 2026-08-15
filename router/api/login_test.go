package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/mocks"
)

type mockCSRFSeeder struct {
	err    error
	called bool
	auth   cookie.Auth
}

func (m *mockCSRFSeeder) Seed(ctx *fasthttp.RequestCtx, auth cookie.Auth) error {
	m.called = true
	m.auth = auth
	return m.err
}

func TestLogin(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	csrf := &mockCSRFSeeder{}
	h := NewHttp(config.Config{WebAppUrl: "https://home.com"}, s, c, csrf)

	tomorrow := time.Now().Add(time.Hour * 24).Format(time.RFC3339)

	ctx := fasthttp.RequestCtx{}
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.SetBodyString(fmt.Sprintf(`{"accessToken":"new_access_token","refreshToken":"new_refresh_token","refreshTokenExpiredAt":"%s"}`, tomorrow))

	err := h.Login(&ctx)

	assert.NoError(t, err)
	assert.Equal(t, `{"redirectUrl":"https://home.com"}`, string(ctx.Response.Body()))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), "new_access_token")
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), "new_refresh_token")
	assert.True(t, csrf.called, "Seed must be called after successful login")
	assert.Equal(t, "new_access_token", csrf.auth.AccessToken)
	assert.Equal(t, "new_refresh_token", csrf.auth.RefreshToken)
}

func TestLoginBadStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	csrf := &mockCSRFSeeder{}
	h := NewHttp(config.Config{WebAppUrl: "https://home.com"}, s, c, csrf)

	ctx := fasthttp.RequestCtx{}
	ctx.Response.SetStatusCode(fasthttp.StatusUnauthorized)
	ctx.Response.SetBodyString(`{"accessToken":"new_access_token","refreshToken":"new_refresh_token"}`)

	err := h.Login(&ctx)

	assert.NoError(t, err)
	assert.NotContains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), "new_access_token")
	assert.NotContains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), "new_refresh_token")
	assert.False(t, csrf.called, "Seed must NOT be called on failed login")
}

func TestLoginInvalidResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	csrf := &mockCSRFSeeder{}
	h := NewHttp(config.Config{WebAppUrl: "https://home.com"}, s, c, csrf)

	ctx := fasthttp.RequestCtx{}
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.SetBodyString(`{"accessToken":"new_access_token","refreshToken":"new_refresh_token`)

	err := h.Login(&ctx)

	assert.Error(t, err)
	assert.NotContains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), "new_access_token")
	assert.False(t, csrf.called, "Seed must NOT be called when response body is invalid")
}

func TestLoginSeedError(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	csrf := &mockCSRFSeeder{err: assert.AnError}
	h := NewHttp(config.Config{WebAppUrl: "https://home.com"}, s, c, csrf)

	tomorrow := time.Now().Add(time.Hour * 24).Format(time.RFC3339)

	ctx := fasthttp.RequestCtx{}
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.SetBodyString(fmt.Sprintf(`{"accessToken":"new_access_token","refreshToken":"new_refresh_token","refreshTokenExpiredAt":"%s"}`, tomorrow))

	// Seed failure is non-fatal: login still succeeds, user recovers via GET /csrf
	err := h.Login(&ctx)

	assert.NoError(t, err)
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), "new_access_token")
	assert.True(t, csrf.called)
}

func TestLoginMalformedRefreshExpiry(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	csrf := &mockCSRFSeeder{}
	h := NewHttp(config.Config{WebAppUrl: "https://home.com"}, s, c, csrf)

	ctx := fasthttp.RequestCtx{}
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
	ctx.Response.SetBodyString(`{"accessToken":"new_access_token","refreshToken":"new_refresh_token","refreshTokenExpiredAt":"not-a-timestamp"}`)

	err := h.Login(&ctx)

	assert.Error(t, err)
	assert.Empty(t, ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName))
	assert.Empty(t, ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName))
	assert.False(t, csrf.called, "Seed must NOT be called when the cookie write fails")
}

// TestAuthSetHandlerMalformedRefreshExpiryReturns502 exercises the full handler chain
// (AuthSetHandler -> Login) to confirm a malformed refreshTokenExpiredAt from the
// backend surfaces as a 502, per writeForwardError/response.ByErrorAndStatus mapping.
func TestAuthSetHandlerMalformedRefreshExpiryReturns502(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	csrf := &mockCSRFSeeder{}
	h := NewHttp(config.Config{WebAppUrl: "https://home.com"}, s, c, csrf)

	c.EXPECT().Verify(gomock.Any()).Return(true, nil)
	s.EXPECT().ForwardRequest(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx *fasthttp.RequestCtx, _ []byte) error {
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.SetBodyString(`{"accessToken":"new_access_token","refreshToken":"new_refresh_token","refreshTokenExpiredAt":"not-a-timestamp"}`)

		return nil
	})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)

	h.AuthSetHandler(&ctx)

	assert.Equal(t, fasthttp.StatusBadGateway, ctx.Response.StatusCode())
	assert.Empty(t, ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName))
}
