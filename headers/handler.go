package headers

import (
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/traces"
)

// Handler is a middleware to write default headers for each response.
func Handler(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		// disable setting automatically header value to identify if Content-Type set by internal handlers
		ctx.Response.Header.SetNoDefaultContentType(true)
		ctx.SetUserValueBytes(clientIpUserValue, findRealClientIP(ctx))

		h(ctx)

		// set default content type if not set earlier
		if val := ctx.Response.Header.ContentType(); val == nil {
			ctx.Response.Header.SetBytesV(ContentType, ContentTypeJson)
		}

		// propagate trace ID to client app for error message details
		if traceId := traces.FindTraceId(ctx); traceId != "" {
			ctx.Response.Header.Set(XCtTraceId, traceId)
		}
	}
}
