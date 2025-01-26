package csrf

import (
	"github.com/valyala/fasthttp"
)

type Handler interface {
	Handler(h fasthttp.RequestHandler) fasthttp.RequestHandler
	RotateTokenHandler(ctx *fasthttp.RequestCtx)
}
