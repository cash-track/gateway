package csrf

import (
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/valyala/fasthttp"
)

// CSRFSeeder seeds the CSRF token for a newly authenticated or refreshed session.
type CSRFSeeder interface {
	Seed(ctx *fasthttp.RequestCtx, auth cookie.Auth) error
}

type Handler interface {
	Handler(h fasthttp.RequestHandler) fasthttp.RequestHandler
	RotateTokenHandler(ctx *fasthttp.RequestCtx)
	Seed(ctx *fasthttp.RequestCtx, auth cookie.Auth) error
}
