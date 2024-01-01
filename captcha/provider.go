package captcha

import "github.com/valyala/fasthttp"

type Provider interface {
	Verify(ctx *fasthttp.RequestCtx) (bool, error)
}
