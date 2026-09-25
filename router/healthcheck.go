package router

import (
	"log/slog"

	"github.com/valyala/fasthttp"
)

var (
	bodyOk     = []byte("ok")
	bodyApiNok = []byte("[api] nok")
)

// LiveHandler consider liveness check successful if request reached the handler.
func (r *Router) LiveHandler(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(bodyOk)
}

// ReadyHandler check all dependency for service readiness.
func (r *Router) ReadyHandler(ctx *fasthttp.RequestCtx) {
	if err := r.api.Healthcheck(); err != nil {
		if r.apiDown.CompareAndSwap(false, true) {
			slog.Warn("API not ready", "error", err)
		}
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetBody(bodyApiNok)

		return
	}

	if r.apiDown.CompareAndSwap(true, false) {
		slog.Info("API ready again")
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(bodyOk)
}
