package router

import (
	"github.com/fasthttp/router"

	"github.com/cash-track/gateway/router/api"
)

func New() *router.Router {
	r := router.New()
	r.POST("/api/auth/login", api.AuthSetHandler)
	r.POST("/api/auth/register", api.AuthSetHandler)
	r.POST("/api/auth/provider/google", api.AuthSetHandler)
	r.POST("/api/auth/logout", api.AuthResetHandler)
	r.ANY("/api/{path:*}", api.FullForwardedHandler)

	return r
}
