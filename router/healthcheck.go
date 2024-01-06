package router

import (
	"log"

	"github.com/valyala/fasthttp"
)

var (
	bodyOk     = []byte("ok")
	bodyApiNok = []byte("[api] nok")
)

// LiveHandler consider liveness check successful if request reached the handler
func (r *Router) LiveHandler(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(bodyOk)
}

// ReadyHandler check all dependency for service readiness
func (r *Router) ReadyHandler(ctx *fasthttp.RequestCtx) {
	if err := r.api.Healthcheck(); err != nil {
		log.Printf("API not ready: %s", err.Error())
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetBody(bodyApiNok)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(bodyOk)
}
