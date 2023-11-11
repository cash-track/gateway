package healthcheck

import (
	"log"

	"github.com/valyala/fasthttp"

	apiClient "github.com/cash-track/gateway/router/api/client"
)

var (
	bodyOk     = []byte("ok")
	bodyApiNok = []byte("[api] nok")
)

// LiveHandler consider liveness check successful if request reached the handler
func LiveHandler(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(bodyOk)
}

// ReadyHandler check all dependency for service readiness
func ReadyHandler(ctx *fasthttp.RequestCtx) {
	if err := apiClient.Healthcheck(); err != nil {
		log.Printf("API not ready: %s", err.Error())
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetBody(bodyApiNok)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(bodyOk)
}
