package api

import (
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/headers/cookie"
)

func Logout(ctx *fasthttp.RequestCtx) error {
	cookie.Auth{}.WriteCookie(ctx)

	b, _ := newWebsiteRedirect().ToJson()

	ctx.Response.SetBody(b)

	return nil
}
