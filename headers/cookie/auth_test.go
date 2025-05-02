package cookie

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cash-track/gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestReadAuthCookie(t *testing.T) {
	for name, test := range map[string]struct {
		AccessToken   string
		RefreshToken  string
		ExpectLogged  bool
		ExpectRefresh bool
	}{
		"OK": {
			AccessToken:   "123",
			RefreshToken:  "123123",
			ExpectLogged:  true,
			ExpectRefresh: true,
		},
		"OnlyLogged": {
			AccessToken:  "123",
			ExpectLogged: true,
		},
		"OnlyRefresh": {
			RefreshToken:  "123123",
			ExpectRefresh: true,
		},
		"Guest": {},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := fasthttp.RequestCtx{}

			ctx.Request.Header.SetCookie(AccessTokenCookieName, test.AccessToken)
			ctx.Request.Header.SetCookie(RefreshTokenCookieName, test.RefreshToken)

			auth := ReadAuthCookie(&ctx)

			assert.Equal(t, test.AccessToken, auth.AccessToken)
			assert.Equal(t, test.RefreshToken, auth.RefreshToken)
			assert.Empty(t, auth.AccessTokenExpiredAt)
			assert.Empty(t, auth.RefreshTokenExpiredAt)
			assert.Equal(t, test.ExpectLogged, auth.IsLogged())
			assert.Equal(t, test.ExpectRefresh, auth.CanRefresh())
		})
	}
}

func TestWriteAuthCookie(t *testing.T) {
	tomorrow := time.Now().Add(time.Hour * 24)
	config.Global.CookieDomain = "test.domain.com"
	config.Global.CookieSecure = true

	for name, test := range map[string]struct {
		Auth                 Auth
		ExpectedAccessToken  string
		ExpectedRefreshToken string
		ExpectedExpire       time.Time
	}{
		"Logged": {
			Auth: Auth{
				AccessToken:           "123",
				AccessTokenExpiredAt:  tomorrow.Format(time.RFC3339),
				RefreshToken:          "123123",
				RefreshTokenExpiredAt: tomorrow.Format(time.RFC3339),
			},
			ExpectedAccessToken:  "123",
			ExpectedRefreshToken: "123123",
			ExpectedExpire:       tomorrow,
		},
		"Guest": {
			Auth:                 Auth{},
			ExpectedAccessToken:  "",
			ExpectedRefreshToken: "",
			ExpectedExpire:       fasthttp.CookieExpireDelete,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := fasthttp.RequestCtx{}

			test.Auth.WriteCookie(&ctx)

			access := ctx.Response.Header.PeekCookie(AccessTokenCookieName)
			assert.Contains(t, string(access), AccessTokenCookieName)
			assert.Contains(t, string(access), "domain=test.domain.com")
			assert.Contains(t, string(access), "path=/")
			assert.Contains(t, string(access), "HttpOnly")
			assert.Contains(t, string(access), "secure")
			assert.Contains(t, string(access), "SameSite=Strict")

			refresh := ctx.Response.Header.PeekCookie(RefreshTokenCookieName)
			assert.Contains(t, string(refresh), RefreshTokenCookieName)
			assert.Contains(t, string(refresh), "domain=test.domain.com")
			assert.Contains(t, string(refresh), "path=/")
			assert.Contains(t, string(refresh), "HttpOnly")
			assert.Contains(t, string(refresh), "secure")
			assert.Contains(t, string(refresh), "SameSite=Strict")

			expire := test.ExpectedExpire.In(time.UTC).Format(time.RFC1123)
			expire = strings.Replace(expire, "UTC", "GMT", 1)

			assert.Contains(t, string(access), test.ExpectedAccessToken)
			assert.Contains(t, string(access), fmt.Sprintf("expires=%s", expire))
			assert.Contains(t, string(refresh), test.ExpectedRefreshToken)
			assert.Contains(t, string(refresh), fmt.Sprintf("expires=%s", expire))

		})
	}
}

func TestAuth_GetOpenTelemetryAttributes(t *testing.T) {
	tomorrow := time.Now().Add(time.Hour * 24).Format(time.RFC3339)

	for name, test := range map[string]struct {
		auth     Auth
		expected map[string]interface{}
	}{
		"LoggedWithRefresh": {
			auth: Auth{
				AccessToken:           "access-123",
				AccessTokenExpiredAt:  tomorrow,
				RefreshToken:          "refresh-123",
				RefreshTokenExpiredAt: tomorrow,
			},
			expected: map[string]interface{}{
				"ct.auth.is_logged":               true,
				"ct.auth.can_refresh":             true,
				"ct.auth.access_token_expire_at":  tomorrow,
				"ct.auth.refresh_token_expire_at": tomorrow,
			},
		},
		"OnlyLogged": {
			auth: Auth{
				AccessToken:          "access-123",
				AccessTokenExpiredAt: tomorrow,
			},
			expected: map[string]interface{}{
				"ct.auth.is_logged":               true,
				"ct.auth.can_refresh":             false,
				"ct.auth.access_token_expire_at":  tomorrow,
				"ct.auth.refresh_token_expire_at": "",
			},
		},
		"Guest": {
			auth: Auth{},
			expected: map[string]interface{}{
				"ct.auth.is_logged":               false,
				"ct.auth.can_refresh":             false,
				"ct.auth.access_token_expire_at":  "",
				"ct.auth.refresh_token_expire_at": "",
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			attrs := test.auth.GetOpenTelemetryAttributes()

			assert.Len(t, attrs, len(test.expected))

			for _, attr := range attrs {
				expectedValue, exists := test.expected[string(attr.Key)]
				assert.True(t, exists, "unexpected attribute key: %s", attr.Key)
				assert.Equal(t, expectedValue, attr.Value.AsInterface())
			}
		})
	}
}
