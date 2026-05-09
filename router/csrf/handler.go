package csrf

import (
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/valyala/fasthttp"
)

type Handler interface {
	Handler(h fasthttp.RequestHandler) fasthttp.RequestHandler
	RotateTokenHandler(ctx *fasthttp.RequestCtx)
	Seed(ctx *fasthttp.RequestCtx, auth cookie.Auth) error
}
