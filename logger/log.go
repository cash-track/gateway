package logger

import (
	"log/slog"
	"time"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/traces"
)

var (
	ignorePaths = map[string]bool{
		"/live":  true,
		"/ready": true,
	}
)

func DebugRequest(req *fasthttp.Request, service string) {
	if config.Global.DebugHttp {
		slog.Debug("debug request", "service", service, "dump", req.String())
	}
}

func DebugResponse(resp *fasthttp.Response, service string) {
	if config.Global.DebugHttp {
		slog.Debug("debug response", "service", service, "dump", resp.String())
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

// FullForwarded logs one structured access-log line per forwarded request. duration is
// the round trip to service only, not gateway-side work (auth/CSRF/captcha). Query string
// and body size are omitted: both are already OpenTelemetry span attributes.
//
// path/method come from the inbound ctx.Request, not the gateway-rewritten outbound one,
// so they match writeForwardError's fields for the same endpoint.
func FullForwarded(
	ctx *fasthttp.RequestCtx,
	resp *fasthttp.Response,
	service string,
	duration time.Duration,
) {
	attrs := []any{
		"trace_id", traces.FindTraceId(ctx),
		"client_ip", headers.GetClientIPFromContext(ctx),
		"method", string(ctx.Request.Header.Method()),
		"path", string(ctx.Request.URI().Path()),
		"status", resp.StatusCode(),
		"duration_ms", duration.Milliseconds(),
		"service", service,
	}

	if resp.StatusCode() >= fasthttp.StatusInternalServerError {
		slog.Warn("request forwarded", attrs...)

		return
	}

	slog.Info("request forwarded", attrs...)
}
