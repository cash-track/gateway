package api

import (
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/headers/cookie"
)

func (h *HttpHandler) Logout(ctx *fasthttp.RequestCtx) {
	// Auth{} is never "logged" (AccessToken == ""), so this always takes the
	// delete branch and cannot fail — error structurally impossible here.
	_ = cookie.Auth{}.WriteCookie(ctx)

	b, _ := h.newWebsiteRedirect().ToJson()

	ctx.Response.SetBody(b)
}
