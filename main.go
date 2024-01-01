package main

import (
	"log"

	prom "github.com/flf2ko/fasthttp-prometheus"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/captcha"
	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/http"
	"github.com/cash-track/gateway/logger"
	"github.com/cash-track/gateway/router"
	apiHandler "github.com/cash-track/gateway/router/api"
	apiService "github.com/cash-track/gateway/service/api"
)

const (
	readBufferSize  = 1024 * 8
	writeBufferSize = 1024 * 8
)

func main() {
	config.Global.Load()

	r := router.New(
		apiHandler.NewHttp(
			config.Global,
			apiService.NewHttp(http.NewFastHttpClient(), config.Global),
			captcha.NewGoogleReCaptchaProvider(http.NewFastHttpClient(), config.Global),
		),
	)
	h := prom.NewPrometheus("http").WrapHandler(r.Router)
	h = headers.Handler(h)
	h = headers.CorsHandler(h)
	h = logger.DebugHandler(h)

	if config.Global.Compress {
		h = fasthttp.CompressHandler(h)
	}

	s := &fasthttp.Server{
		Handler:         h,
		ReadBufferSize:  readBufferSize,
		WriteBufferSize: writeBufferSize,
	}

	if config.Global.HttpsEnabled {
		startTls(s)
	} else {
		start(s)
	}
}

func start(s *fasthttp.Server) {
	log.Printf("Listening on HTTP %s", config.Global.Address)

	if err := s.ListenAndServe(config.Global.Address); err != nil {
		log.Fatalf("Error in HTTP server: %v", err)
	}
}

func startTls(s *fasthttp.Server) {
	log.Printf("Listening on HTTPS %s", config.Global.Address)

	if err := s.ListenAndServeTLS(config.Global.Address, config.Global.HttpsCrt, config.Global.HttpsKey); err != nil {
		log.Fatalf("Error in HTTPS server: %v", err)
	}
}
