package headers

import (
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
	CorsIgnorePaths = map[string]bool{
		"/live":  true,
		"/ready": true,
	}
)

// CorsHandler is a middleware to write default CORS headers if no forwarded
func CorsHandler(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		h(ctx)

		if validateCorsOrigin(ctx) {
			writeCorsAllowedHeaders(ctx)
		}
	}
}

func validateCorsOrigin(ctx *fasthttp.RequestCtx) bool {
	if _, ok := CorsIgnorePaths[string(ctx.Request.URI().Path())]; ok {
		return false
	}

	clientIp := GetClientIPFromContext(ctx)
	if val := ctx.Response.Header.Peek(AccessControlAllowOrigin); val != nil {
		// CORS headers were already configured by upstream
		log.Printf("[%s] CORS validation for origin by upstream: %s", clientIp, val)
		return false
	}

	origin := strings.ToLower(string(ctx.Request.Header.Peek(Origin)))
	_, ok := config.Global.CorsAllowedOrigins[origin]

	log.Printf("[%s] CORS validation for origin %s by gateway: %v", clientIp, origin, ok)

	return ok
}

func writeCorsAllowedHeaders(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.SetBytesV(AccessControlAllowOrigin, ctx.Request.Header.Peek(Origin))
	ctx.Response.Header.Set(AccessControlAllowMethods, strings.Join(CorsAllowedMethods, ", "))
	ctx.Response.Header.Set(AccessControlAllowHeaders, strings.Join(CorsAllowedHeaders, ", "))
	ctx.Response.Header.Set(AccessControlAllowCredentials, "true")
}
