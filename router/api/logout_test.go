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

func TestLogout(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{
		WebsiteUrl: "https://test.com",
	}, s, c)

	ctx := fasthttp.RequestCtx{}

	h.Logout(&ctx)

	assert.Equal(t, `{"redirectUrl":"https://test.com"}`, string(ctx.Response.Body()))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), fmt.Sprintf("%s=;", cookie.AccessTokenCookieName))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), fmt.Sprintf("%s=;", cookie.RefreshTokenCookieName))
}
