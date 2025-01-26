package cookie

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestReadCSRFCookie(t *testing.T) {
	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetCookie(AccessTokenCookieName, "access_token")
	ctx.Request.Header.SetCookie(CsrfTokenCookieName, "csrf_token")

	csrf := ReadCSRFCookie(&ctx)

	assert.Equal(t, "csrf_token", csrf.Token)
	assert.Equal(t, true, csrf.Auth.IsLogged())
}

func TestWriteCSRFCookie(t *testing.T) {
	for name, test := range map[string]struct {
		csrf          CSRF
		expectedToken string
	}{
		"Logged": {
			csrf: CSRF{
				Auth: Auth{
					AccessToken: "access_token",
				},
				Token: "csrf_token",
			},
			expectedToken: "csrf_token",
		},
		"Guest": {
			csrf: CSRF{
				Token: "csrf_token",
			},
			expectedToken: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := fasthttp.RequestCtx{}

			test.csrf.WriteCookie(&ctx)

			token := ctx.Response.Header.PeekCookie(CsrfTokenCookieName)
			assert.Contains(t, string(token), test.expectedToken)
		})
	}
}
