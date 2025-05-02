package main

import (
	"context"
	"log"
	"time"

	prom "github.com/flf2ko/fasthttp-prometheus"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/captcha"
	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/http/retryhttp"
	"github.com/cash-track/gateway/logger"
	"github.com/cash-track/gateway/router"
	apiHandler "github.com/cash-track/gateway/router/api"
	csrfHandler "github.com/cash-track/gateway/router/csrf"
	apiService "github.com/cash-track/gateway/service/api"
	"github.com/cash-track/gateway/traces"
)

const (
	readBufferSize            = 1024 * 8
	writeBufferSize           = 1024 * 8
	redisClientConnectTimeout = 5 * time.Second
)

func main() {
	ctx := context.Background()

	config.Global.Load()

	if _, tracerClose, err := traces.NewTracer(ctx); err != nil {
		log.Fatalf("Error creating OpenTelemetry tracer: %v", err)
	} else {
		defer tracerClose()
	}

	redisClient := getRedisClient()
	csrf := csrfHandler.NewRedisHandler(redisClient)

	r := router.New(
		apiHandler.NewHttp(
			config.Global,
			apiService.NewHttp(retryhttp.NewFastHttpRetryClient(), config.Global),
			captcha.NewGoogleReCaptchaProvider(retryhttp.NewFastHttpRetryClient(), config.Global),
		),
		csrf,
	)
	h := prom.NewPrometheus("http").WrapHandler(r.Router)
	h = headers.Handler(h)
	if config.Global.CsrfEnabled {
		h = csrf.Handler(h)
	}
	h = headers.CorsHandler(h)
	h = logger.DebugHandler(h)
	h = traces.TraceHandler(h)

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

func getRedisClient() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: config.Global.RedisConnection,
	})

	if err := redisotel.InstrumentTracing(client); err != nil {
		log.Fatalf("Error configuring OTEL instrument to redis: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisClientConnectTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatalf("Error connecting to redis: %v", err)
	}

	log.Printf("Connected to Redis at %s\n", config.Global.RedisConnection)

	return client
}
