package api

import (
	"strings"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/http/retryhttp"
	"github.com/cash-track/gateway/router/csrf"
)

const (
	ServiceId         = "API"
	httpReadTimeout   = 5 * time.Second
	httpWriteTimeout  = 5 * time.Second
	httpRetryAttempts = uint(2)
)

var methodsWithBody = map[string]bool{
	fasthttp.MethodPost:  true,
	fasthttp.MethodPut:   true,
	fasthttp.MethodPatch: true,
}

type Service interface {
	ForwardRequest(ctx *fasthttp.RequestCtx, body []byte) error
	Healthcheck() error
}

type HttpService struct {
	http    retryhttp.Client
	config  config.Config
	csrf    csrf.CSRFSeeder
	breaker *gobreaker.CircuitBreaker[struct{}]
}

func NewHttp(
	http retryhttp.Client,
	config config.Config,
	csrf csrf.CSRFSeeder,
	breaker *gobreaker.CircuitBreaker[struct{}],
) *HttpService {
	http.WithReadTimeout(httpReadTimeout)
	http.WithWriteTimeout(httpWriteTimeout)
	http.WithRetryAttempts(httpRetryAttempts)

	return &HttpService{
		http:    http,
		config:  config,
		csrf:    csrf,
		breaker: breaker,
	}
}

func (s *HttpService) setRequestURI(dest *fasthttp.URI, path []byte) {
	_ = dest.Parse([]byte(s.config.ApiUrl), nil)
	dest.SetScheme(s.config.ApiURI.Scheme)
	dest.SetHost(s.config.ApiURI.Host)
	dest.SetPathBytes(path)
}

func (s *HttpService) copyRequestURI(src, dest *fasthttp.URI) {
	path := "/v1" + strings.TrimPrefix(string(src.PathOriginal()), "/api")
	s.setRequestURI(dest, []byte(path))
	dest.SetQueryStringBytes(src.QueryString())
}
