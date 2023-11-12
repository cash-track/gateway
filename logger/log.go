package logger

import (
	"fmt"
	"log"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
)

func DebugRequest(req *fasthttp.Request, service string) {
	if config.Global.DebugHttp {
		log.Printf("DEBUG REQ %s\n%s", service, req)
	}
}

func DebugResponse(resp *fasthttp.Response, service string) {
	if config.Global.DebugHttp {
		log.Printf("DEBUG RESP %s\n%s", service, resp)
	}
}

func DebugHandler(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		DebugRequest(&ctx.Request, "")
		h(ctx)
		DebugResponse(&ctx.Response, "")
	}
}

func FullForwarded(ctx *fasthttp.RequestCtx, req *fasthttp.Request, resp *fasthttp.Response, service string) {
	line := fmt.Sprintf("[%s] %s %s", headers.GetClientIPFromContext(ctx), req.Header.Method(), req.URI().Path())

	if req.URI().QueryArgs().Len() > 0 {
		line += fmt.Sprintf("?%s", req.URI().QueryString())
	}

	if req.Body() != nil || len(req.Body()) > 0 {
		line += fmt.Sprintf(" (body %db)", len(req.Body()))
	}

	line += fmt.Sprintf(" => %s resp %d (%db)", service, resp.StatusCode(), len(resp.Body()))

	log.Printf(line)
}
