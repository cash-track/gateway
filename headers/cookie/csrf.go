package cookie

import (
	"time"

	"github.com/valyala/fasthttp"
)

const (
	CsrfTokenCookieName = "cshtrkcsrf"
	CsrfTokenTtl        = time.Minute * 10
)

type CSRF struct {
	Auth  Auth
	Token string
}

func ReadCSRFCookie(ctx *fasthttp.RequestCtx) CSRF {
	csrf := CSRF{
		Auth: ReadAuthCookie(ctx),
	}

	if val := ctx.Request.Header.Cookie(CsrfTokenCookieName); val != nil {
		csrf.Token = string(val)
	}

	return csrf
}

func (c CSRF) WriteCookie(ctx *fasthttp.RequestCtx) {
	if !c.Auth.IsLogged() {
		ctx.Response.Header.SetCookie(newCookie(CsrfTokenCookieName, "", fasthttp.CookieExpireDelete))
		return
	}

	ctx.Response.Header.SetCookie(newCookie(CsrfTokenCookieName, c.Token, time.Now().Add(CsrfTokenTtl)))
}
