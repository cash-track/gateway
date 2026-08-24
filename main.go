package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	prom "github.com/flf2ko/fasthttp-prometheus"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/captcha"
	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/http/retryhttp"
	"github.com/cash-track/gateway/jwks"
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
	// Debug level so DebugRequest/DebugResponse (gated by config.Global.DebugHttp) can emit.
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler).With("component", "gateway"))

	ctx := context.Background()

	config.Global.Load()

	if _, tracerClose, err := traces.NewTracer(ctx); err != nil {
		slog.Error("error creating OpenTelemetry tracer", "error", err)
		os.Exit(1)
	} else {
		defer tracerClose()
	}

	jwksProvider := jwks.NewHttp(retryhttp.NewFastHttpRetryClient(), config.Global)
	jwksProvider.Start(ctx)

	redisClient := getRedisClient()
	csrf := csrfHandler.NewRedisHandler(redisClient, jwksProvider)
	breaker := apiService.NewBreaker()
	apiService.RegisterBreakerMetrics(breaker)

	r := router.New(
		apiHandler.NewHttp(
			config.Global,
			apiService.NewHttp(retryhttp.NewFastHttpRetryClient(), config.Global, csrf, breaker),
			captcha.NewGoogleReCaptchaProvider(retryhttp.NewFastHttpRetryClient(), config.Global),
			csrf,
		),
		csrf,
	)
	h := buildHandler(prom.NewPrometheus("http").WrapHandler(r.Router), csrf)

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

// buildHandler chains the middleware applied to every request, outermost first:
// traces -> logger -> cors -> headers -> csrf (if enabled) -> inner.
//
// headers must wrap csrf, not the reverse: csrf short-circuits a validation failure with a
// 417 without calling its inner handler, which would leave that response with no trace ID
// and no provenance headers.
func buildHandler(inner fasthttp.RequestHandler, csrf csrfHandler.Handler) fasthttp.RequestHandler {
	h := inner
	if config.Global.CsrfEnabled {
		h = csrf.Handler(h)
	}
	h = headers.Handler(h)
	h = headers.CorsHandler(h)
	h = logger.DebugHandler(h)
	h = traces.TraceHandler(h)

	if config.Global.Compress {
		h = fasthttp.CompressHandler(h)
	}

	return h
}

func start(s *fasthttp.Server) {
	slog.Info("listening on HTTP", "address", config.Global.Address)

	if err := s.ListenAndServe(config.Global.Address); err != nil {
		slog.Error("error in HTTP server", "error", err)
		os.Exit(1)
	}
}

func startTls(s *fasthttp.Server) {
	slog.Info("listening on HTTPS", "address", config.Global.Address)

	if err := s.ListenAndServeTLS(config.Global.Address, config.Global.HttpsCrt, config.Global.HttpsKey); err != nil {
		slog.Error("error in HTTPS server", "error", err)
		os.Exit(1)
	}
}

func getRedisClient() *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: config.Global.RedisConnection,
	})

	if err := redisotel.InstrumentTracing(client); err != nil {
		slog.Error("error configuring OTEL instrument to redis", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisClientConnectTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		slog.Error("error connecting to redis", "error", err)
		os.Exit(1)
	}

	slog.Info("connected to redis", "address", config.Global.RedisConnection)

	return client
}
