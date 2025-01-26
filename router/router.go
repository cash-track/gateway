package router

import (
	"github.com/fasthttp/router"

	"github.com/cash-track/gateway/router/api"
	"github.com/cash-track/gateway/router/csrf"
)

type Router struct {
	*router.Router
	api  api.Handler
	csrf csrf.Handler
}

func New(api api.Handler, csrf csrf.Handler) *Router {
	r := &Router{
		Router: router.New(),
		api:    api,
		csrf:   csrf,
	}
	r.register()
	return r
}

func (r *Router) register() {
	r.ANY("/live", r.LiveHandler)
	r.ANY("/ready", r.ReadyHandler)
	r.GET("/csrf", r.csrf.RotateTokenHandler)

	r.POST("/api/auth/login", r.api.AuthSetHandler)
	r.POST("/api/auth/login/passkey", r.api.AuthSetHandler)
	r.POST("/api/auth/login/passkey/init", r.api.CaptchaVerifyHandler)
	r.POST("/api/auth/register", r.api.AuthSetHandler)
	r.POST("/api/auth/provider/google", r.api.AuthSetHandler)
	r.POST("/api/auth/logout", r.api.AuthResetHandler)
	r.ANY("/api/{path:*}", r.api.FullForwardedHandler)
}
