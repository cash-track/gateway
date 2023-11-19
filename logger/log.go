package logger

import (
	"fmt"
	"log"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
)

var (
	ignorePaths = map[string]bool{
		"/live":  true,
		"/ready": true,
	}
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
		_, ignore := ignorePaths[string(ctx.Request.URI().Path())]

		if !ignore {
			DebugRequest(&ctx.Request, "")
		}

		h(ctx)

		if !ignore {
			DebugResponse(&ctx.Response, "")
		}
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

	log.Print(line)
}
