package api

import (
	"encoding/json"
	"fmt"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/http"
	"github.com/cash-track/gateway/router/api/client"
	"github.com/cash-track/gateway/router/captcha"
	"github.com/cash-track/gateway/router/response"
)

var allowedMethods = map[string]bool{
	fasthttp.MethodGet:     true,
	fasthttp.MethodPost:    true,
	fasthttp.MethodPut:     true,
	fasthttp.MethodPatch:   true,
	fasthttp.MethodDelete:  true,
	fasthttp.MethodOptions: true,
}

func AuthSetHandler(ctx *fasthttp.RequestCtx) {
	reCaptcha := captcha.NewGoogleReCaptchaProvider(http.NewFastHttpClient(), config.Global)

	if ok, err := reCaptcha.Verify(ctx); err != nil || !ok {
		if err != nil {
			response.NewCaptchaErrorResponse(err).Write(ctx)
			return
		} else {
			response.NewCaptchaBadResponse().Write(ctx)
			return
		}
	}

	FullForwardedHandler(ctx)

	if err := Login(ctx); err != nil {
		response.ByError(err).Write(ctx)
	}
}

func AuthResetHandler(ctx *fasthttp.RequestCtx) {
	auth := cookie.ReadAuthCookie(ctx)

	FullForwardedHandlerWithBody(ctx, cookie.Auth{
		RefreshToken: auth.RefreshToken,
	})

	if err := Logout(ctx); err != nil {
		response.ByError(err).Write(ctx)
	}
}

func FullForwardedHandler(ctx *fasthttp.RequestCtx) {
	if _, ok := allowedMethods[string(ctx.Request.Header.Method())]; !ok {
		response.ByErrorAndStatus(
			fmt.Errorf("request method %s is not allowed", ctx.Request.Header.Method()),
			fasthttp.StatusBadRequest,
		).Write(ctx)
		return
	}

	err := client.ForwardRequest(ctx, nil)
	if err != nil {
		response.ByErrorAndStatus(err, fasthttp.StatusBadGateway).Write(ctx)
		return
	}
}

func FullForwardedHandlerWithBody(ctx *fasthttp.RequestCtx, body interface{}) {
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

	if err := client.ForwardRequest(ctx, b); err != nil {
		response.ByErrorAndStatus(err, fasthttp.StatusBadGateway).Write(ctx)
		return
	}
}
