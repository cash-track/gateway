package api

import (
	"fmt"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/headers/cookie"
)

func Logout(ctx *fasthttp.RequestCtx) error {
	cookie.Auth{}.WriteCookie(ctx)

	b, err := newWebsiteRedirect().ToJson()
	if err != nil {
		return fmt.Errorf("login response build error: %w", err)
	}

	ctx.Response.SetBody(b)

	return nil
}
