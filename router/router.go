package router

import (
	"github.com/fasthttp/router"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/router/api"
)

type Router struct {
	*router.Router
	config config.Config
}

func New(config config.Config) *Router {
	r := &Router{
		Router: router.New(),
		config: config,
	}
	r.register()
	return r
}

func (r *Router) register() {
	r.ANY("/live", r.LiveHandler)
	r.ANY("/ready", r.ReadyHandler)

	r.POST("/api/auth/login", api.AuthSetHandler)
	r.POST("/api/auth/register", api.AuthSetHandler)
	r.POST("/api/auth/provider/google", api.AuthSetHandler)
	r.POST("/api/auth/logout", api.AuthResetHandler)
	r.ANY("/api/{path:*}", api.FullForwardedHandler)
}
