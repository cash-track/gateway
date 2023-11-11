package main

import (
	"log"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/logger"
	"github.com/cash-track/gateway/router"
)

func main() {
	config.Global.Load()

	h := router.New().Handler
	h = headers.Handler(h)
	h = headers.CorsHandler(h)
	h = logger.DebugHandler(h)

	if config.Global.Compress {
		h = fasthttp.CompressHandler(h)
	}

	if config.Global.HttpsEnabled {
		startTls(h)
	} else {
		start(h)
	}
}

func start(h fasthttp.RequestHandler) {
	log.Printf("Listening on HTTP %s", config.Global.Address)

	if err := fasthttp.ListenAndServe(config.Global.Address, h); err != nil {
		log.Fatalf("Error in HTTP server: %v", err)
	}
}

func startTls(h fasthttp.RequestHandler) {
	log.Printf("Listening on HTTPS %s", config.Global.Address)

	if err := fasthttp.ListenAndServeTLS(config.Global.Address, config.Global.HttpsCrt, config.Global.HttpsKey, h); err != nil {
		log.Fatalf("Error in HTTPS server: %v", err)
	}
}
