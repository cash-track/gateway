package api

import (
	"encoding/json"
	"fmt"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/captcha"
	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/router/response"
	"github.com/cash-track/gateway/service/api"
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
	FullForwardedHandler(ctx *fasthttp.RequestCtx)
	Healthcheck() error
}

type HttpHandler struct {
	config  config.Config
	captcha captcha.Provider
	service api.Service
}

func NewHttp(config config.Config, service api.Service, captcha captcha.Provider) *HttpHandler {
	return &HttpHandler{
		config:  config,
		captcha: captcha,
		service: service,
	}
}

func (h *HttpHandler) AuthSetHandler(ctx *fasthttp.RequestCtx) {
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

	if err := h.Login(ctx); err != nil {
		response.ByErrorAndStatus(err, fasthttp.StatusBadGateway).Write(ctx)
	}
}

func (h *HttpHandler) AuthResetHandler(ctx *fasthttp.RequestCtx) {
	auth := cookie.ReadAuthCookie(ctx)

	h.FullForwardedHandlerWithBody(ctx, cookie.Auth{
		RefreshToken: auth.RefreshToken,
	})

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
		response.ByErrorAndStatus(err, fasthttp.StatusBadGateway).Write(ctx)
		return
	}
}

func (h *HttpHandler) FullForwardedHandlerWithBody(ctx *fasthttp.RequestCtx, body interface{}) {
	if _, ok := allowedMethods[string(ctx.Request.Header.Method())]; !ok {
		response.ByErrorAndStatus(
			fmt.Errorf("request method %s is not allowed", ctx.Request.Header.Method()),
			fasthttp.StatusBadRequest,
		).Write(ctx)
		return
	}

	b, err := json.Marshal(body)
	if err != nil {
		response.ByError(err).Write(ctx)
		return
	}

	if err := h.service.ForwardRequest(ctx, b); err != nil {
		response.ByErrorAndStatus(err, fasthttp.StatusBadGateway).Write(ctx)
		return
	}
}

func (h *HttpHandler) Healthcheck() error {
	return h.service.Healthcheck()
}
