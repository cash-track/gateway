package headers

import (
	"bytes"
	"log/slog"
	"strings"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
)

// corsMaxAge is the Access-Control-Max-Age value (seconds) advertised on preflight
// responses.
const corsMaxAge = "600"

var (
	CorsAllowedMethods = []string{
		fasthttp.MethodGet,
		fasthttp.MethodPost,
		fasthttp.MethodPut,
		fasthttp.MethodPatch,
		fasthttp.MethodDelete,
		fasthttp.MethodOptions,
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

// isPreflightRequest reports whether ctx is a genuine CORS preflight request per the
// Fetch spec: an OPTIONS request carrying Access-Control-Request-Method. An OPTIONS
// request without that header is not a preflight and must be treated like any other request.
func isPreflightRequest(ctx *fasthttp.RequestCtx) bool {
	return string(ctx.Request.Header.Method()) == fasthttp.MethodOptions &&
		len(ctx.Request.Header.Peek(AccessControlRequestMethod)) > 0
}

// CorsHandler answers a genuine preflight request directly — routing/csrf/captcha/
// forwarding never see it. Any other request is handled normally and then decorated
// with CORS response headers.
func CorsHandler(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		if !isHealthPath(ctx) && isPreflightRequest(ctx) {
			writePreflightResponse(ctx)

			return
		}

		h(ctx)

		if isAllowedOrigin(ctx) {
			writeActualCorsHeaders(ctx)
		}
	}
}

// writePreflightResponse answers a preflight request directly, without invoking the
// inner handler. Always responds 204. Origin allowed: full CORS header set, dynamically
// reflecting Access-Control-Request-Headers. Origin not allowed: same 204, no CORS headers.
func writePreflightResponse(ctx *fasthttp.RequestCtx) {
	ctx.Response.SetStatusCode(fasthttp.StatusNoContent)

	origin := requestOrigin(ctx)
	allowed := config.Global.CorsAllowedOrigins[origin]

	debugCorsOrigin(ctx, "preflight", origin, allowed)

	if !allowed {
		return
	}

	ctx.Response.Header.SetBytesV(AccessControlAllowOrigin, []byte(origin))
	addVary(&ctx.Response.Header, Origin)
	ctx.Response.Header.Set(AccessControlAllowCredentials, "true")
	ctx.Response.Header.Set(AccessControlAllowMethods, strings.Join(CorsAllowedMethods, ","))
	ctx.Response.Header.SetBytesV(AccessControlAllowHeaders, ctx.Request.Header.Peek(AccessControlRequestHeaders))
	addVary(&ctx.Response.Header, AccessControlRequestHeaders)
	ctx.Response.Header.Set(AccessControlMaxAge, corsMaxAge)
}

// isAllowedOrigin reports whether ctx is eligible for CORS decoration: not a health
// path, and its Origin header is in the allow-list.
func isAllowedOrigin(ctx *fasthttp.RequestCtx) bool {
	if isHealthPath(ctx) {
		return false
	}

	origin := requestOrigin(ctx)
	allowed := config.Global.CorsAllowedOrigins[origin]

	debugCorsOrigin(ctx, "actual", origin, allowed)

	return allowed
}

// writeActualCorsHeaders decorates an actual (non-preflight) response for an allowed
// origin: Allow-Origin, Vary, Allow-Credentials and the exposed-header list.
// Allow-Methods/Allow-Headers/Max-Age are preflight-only and not set here.
func writeActualCorsHeaders(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.SetBytesV(AccessControlAllowOrigin, []byte(requestOrigin(ctx)))
	addVary(&ctx.Response.Header, Origin)
	ctx.Response.Header.Set(AccessControlAllowCredentials, "true")
	ctx.Response.Header.Set(AccessControlExposeHeaders, strings.Join(CorsExposedHeaders, ","))
}

func requestOrigin(ctx *fasthttp.RequestCtx) string {
	return strings.ToLower(string(ctx.Request.Header.Peek(Origin)))
}

func debugCorsOrigin(ctx *fasthttp.RequestCtx, stage, origin string, allowed bool) {
	if !config.Global.DebugHttp {
		return
	}

	slog.Debug("CORS validation for origin by gateway",
		"stage", stage, "client_ip", GetClientIPFromContext(ctx), "origin", origin, "allowed", allowed)
}

// addVary appends value to the Vary header if not already present, following HTTP's
// comma-separated multi-value convention (mirrors fasthttp's own unexported
// addVaryBytes helper, used internally for Accept-Encoding).
func addVary(header *fasthttp.ResponseHeader, value string) {
	current := header.Peek(Vary)
	if len(current) == 0 {
		header.Set(Vary, value)

		return
	}

	if !bytes.Contains(current, []byte(value)) {
		header.SetBytesV(Vary, append(append(bytes.Clone(current), ", "...), value...))
	}
}
