package api

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers/cookie"
)

func TestLogout(t *testing.T) {
	config.Global.WebsiteUrl = "https://test.com"

	ctx := fasthttp.RequestCtx{}

	err := Logout(&ctx)

	assert.NoError(t, err)
	assert.Equal(t, `{"redirectUrl":"https://test.com"}`, string(ctx.Response.Body()))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), fmt.Sprintf("%s=;", cookie.AccessTokenCookieName))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), fmt.Sprintf("%s=;", cookie.RefreshTokenCookieName))
}
