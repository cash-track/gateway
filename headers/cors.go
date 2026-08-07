package headers

import (
	"bytes"
	"log"
	"strings"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
)

var (
	CorsAllowedMethods = []string{
		fasthttp.MethodGet,
		fasthttp.MethodPost,
		fasthttp.MethodPut,
		fasthttp.MethodPatch,
		fasthttp.MethodDelete,
		fasthttp.MethodOptions,
	}
	CorsAllowedHeaders = []string{
		ContentType,
		XCtCaptchaChallenge,
		"*",
	}
	CorsExposedHeaders = []string{
		XCtTraceId,
		XCtGatewayVersion,
		XCtGatewaySha,
		XCtApiVersion,
		XCtApiSha,
	}
	// healthPaths lists probe endpoints excluded from CORS and security response headers.
	healthPaths = map[string]bool{
		"/live":  true,
		"/ready": true,
	}
)

// isHealthPath reports whether ctx targets a health probe endpoint. Matches on
// PathOriginal, the raw undecoded path fasthttp/router dispatches on, so an
// encoded lookalike (a 404 to the router) isn't treated as the real probe.
func isHealthPath(ctx *fasthttp.RequestCtx) bool {
	return healthPaths[string(ctx.Request.URI().PathOriginal())]
}

// CorsHandler is a middleware to write default CORS headers if no forwarded.
func CorsHandler(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		h(ctx)

		if validateCorsOrigin(ctx) {
			writeCorsAllowedHeaders(ctx)
		}
	}
}

func validateCorsOrigin(ctx *fasthttp.RequestCtx) bool {
	if isHealthPath(ctx) {
		return false
	}

	clientIp := GetClientIPFromContext(ctx)
	if val := ctx.Response.Header.Peek(AccessControlAllowOrigin); val != nil {
		// CORS headers were already configured by upstream
		if config.Global.DebugHttp {
			log.Printf("[%s] CORS validation for origin by upstream: %s", clientIp, val)
		}

		return false
	}

	origin := strings.ToLower(string(ctx.Request.Header.Peek(Origin)))
	_, ok := config.Global.CorsAllowedOrigins[origin]

	if config.Global.DebugHttp {
		log.Printf("[%s] CORS validation for origin %s by gateway: %v", clientIp, origin, ok)
	}

	return ok
}

func writeCorsAllowedHeaders(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.SetBytesV(AccessControlAllowOrigin, bytes.ToLower(ctx.Request.Header.Peek(Origin)))
	ctx.Response.Header.Set(AccessControlAllowMethods, strings.Join(CorsAllowedMethods, ","))
	ctx.Response.Header.Set(AccessControlAllowHeaders, strings.Join(CorsAllowedHeaders, ","))
	ctx.Response.Header.Set(AccessControlAllowCredentials, "true")
	ctx.Response.Header.Set(AccessControlExposeHeaders, strings.Join(CorsExposedHeaders, ","))

	if string(ctx.Request.Header.Method()) == fasthttp.MethodOptions {
		ctx.Response.Header.SetStatusCode(fasthttp.StatusOK)
	}
}
