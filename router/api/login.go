package api

import (
	"encoding/json"
	"fmt"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/headers/cookie"
)

func Login(ctx *fasthttp.RequestCtx) error {
	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		return nil
	}

	auth := cookie.Auth{}
	if err := json.Unmarshal(ctx.Response.Body(), &auth); err != nil {
		return fmt.Errorf("login response body invalid: %w", err)
	}

	auth.WriteCookie(ctx)

	b, err := newWebAppRedirect().ToJson()
	if err != nil {
		return fmt.Errorf("login response build error: %w", err)
	}

	ctx.Response.SetBody(b)

	return nil
}
