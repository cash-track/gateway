package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/captcha"
	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/router/csrf"
	"github.com/cash-track/gateway/router/response"
	"github.com/cash-track/gateway/service/api"
	"github.com/cash-track/gateway/traces"
)

var allowedMethods = map[string]bool{
	fasthttp.MethodGet:     true,
	fasthttp.MethodPost:    true,
	fasthttp.MethodPut:     true,
	fasthttp.MethodPatch:   true,
	fasthttp.MethodDelete:  true,
	fasthttp.MethodOptions: true,
}

type Handler interface {
	AuthSetHandler(ctx *fasthttp.RequestCtx)
	AuthResetHandler(ctx *fasthttp.RequestCtx)
	CaptchaVerifyHandler(ctx *fasthttp.RequestCtx)
	FullForwardedHandler(ctx *fasthttp.RequestCtx)
	Healthcheck() error
}

type HttpHandler struct {
	config  config.Config
	captcha captcha.Provider
	service api.Service
	csrf    csrf.CSRFSeeder
}

func NewHttp(config config.Config, service api.Service, captcha captcha.Provider, csrf csrf.CSRFSeeder) *HttpHandler {
	return &HttpHandler{
		config:  config,
		captcha: captcha,
		service: service,
		csrf:    csrf,
	}
}

func (h *HttpHandler) AuthSetHandler(ctx *fasthttp.RequestCtx) {
	h.CaptchaVerifyHandler(ctx)

	if err := h.Login(ctx); err != nil {
		response.ByErrorAndStatus(err, fasthttp.StatusBadGateway).Write(ctx)
	}
}

func (h *HttpHandler) CaptchaVerifyHandler(ctx *fasthttp.RequestCtx) {
	if ok, err := h.captcha.Verify(ctx); err != nil || !ok {
		if err != nil {
			response.NewCaptchaErrorResponse(err).Write(ctx)

			return
		} else {
			response.NewCaptchaBadResponse().Write(ctx)

			return
		}
	}

	h.FullForwardedHandler(ctx)
}

func (h *HttpHandler) AuthResetHandler(ctx *fasthttp.RequestCtx) {
	auth := cookie.ReadAuthCookie(ctx)

	err := h.FullForwardedHandlerWithBody(ctx, cookie.Auth{
		RefreshToken: auth.RefreshToken,
	})
	if err != nil {
		slog.Warn("logout: forwarding to backend failed, clearing cookies locally anyway",
			"trace_id", traces.FindTraceId(ctx),
			"error", err,
		)
	}

	// The response is gateway-authored, so drop everything CopyFromResponse may have
	// copied from the backend (Retry-After, X-Ratelimit-*, CORS, ...) rather than
	// deleting headers one by one as that list grows.
	//
	// Reset() also clears the noDefaultContentType flag headers.Handler relies on, and
	// leaves ContentType() non-nil but empty, so its fallback never fires. Both are
	// re-asserted below.
	ctx.Response.Reset()
	ctx.Response.Header.SetNoDefaultContentType(true)
	ctx.Response.Header.SetContentTypeBytes(headers.ContentTypeJson)
	ctx.Response.SetStatusCode(fasthttp.StatusOK)

	h.Logout(ctx)
}

func (h *HttpHandler) FullForwardedHandler(ctx *fasthttp.RequestCtx) {
	if _, ok := allowedMethods[string(ctx.Request.Header.Method())]; !ok {
		response.ByErrorAndStatus(
			fmt.Errorf("request method %s is not allowed", ctx.Request.Header.Method()),
			fasthttp.StatusBadRequest,
		).Write(ctx)

		return
	}

	err := h.service.ForwardRequest(ctx, nil)
	if err != nil {
		writeForwardError(ctx, err)

		return
	}
}

// FullForwardedHandlerWithBody forwards ctx with a JSON-marshaled body. It writes the
// error response to ctx and also returns the failure, for callers that react to it.
func (h *HttpHandler) FullForwardedHandlerWithBody(ctx *fasthttp.RequestCtx, body any) error {
	if _, ok := allowedMethods[string(ctx.Request.Header.Method())]; !ok {
		err := fmt.Errorf("request method %s is not allowed", ctx.Request.Header.Method())
		response.ByErrorAndStatus(err, fasthttp.StatusBadRequest).Write(ctx)

		return err
	}

	b, err := json.Marshal(body)
	if err != nil {
		response.ByError(err).Write(ctx)

		return err
	}

	if err := h.service.ForwardRequest(ctx, b); err != nil {
		writeForwardError(ctx, err)

		return err
	}

	return nil
}

// writeForwardError maps a ForwardRequest failure to a response: 503 with a Retry-After
// hint when the circuit breaker is open, 502 for any other transport error.
func writeForwardError(ctx *fasthttp.RequestCtx, err error) {
	attrs := []any{
		"trace_id", traces.FindTraceId(ctx),
		"method", string(ctx.Request.Header.Method()),
		"path", string(ctx.Request.URI().Path()),
		"error", err,
	}

	if errors.Is(err, api.ErrCircuitOpen) {
		slog.Warn("forward request rejected: circuit breaker open", attrs...)

		ctx.Response.Header.Set(headers.RetryAfter, strconv.Itoa(api.RetryAfterSeconds))
		response.ByErrorAndStatus(err, fasthttp.StatusServiceUnavailable).Write(ctx)

		return
	}

	slog.Error("forward request failed", attrs...)

	response.ByErrorAndStatus(err, fasthttp.StatusBadGateway).Write(ctx)
}

func (h *HttpHandler) Healthcheck() error {
	return h.service.Healthcheck()
}
