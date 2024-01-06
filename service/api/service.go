package api

import (
	"strings"
	"time"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/http"
)

const (
	ServiceId        = "API"
	httpReadTimeout  = 5 * time.Second
	httpWriteTimeout = 5 * time.Second
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
	http   http.Client
	config config.Config
}

func NewHttp(http http.Client, config config.Config) *HttpService {
	http.WithReadTimeout(httpReadTimeout)
	http.WithWriteTimeout(httpWriteTimeout)

	return &HttpService{
		http:   http,
		config: config,
	}
}

func (s *HttpService) setRequestURI(dest *fasthttp.URI, path []byte) {
	_ = dest.Parse([]byte(s.config.ApiUrl), nil)
	dest.SetScheme(s.config.ApiURI.Scheme)
	dest.SetHost(s.config.ApiURI.Host)
	dest.SetPathBytes(path)
}

func (s *HttpService) copyRequestURI(src, dest *fasthttp.URI) {
	path := strings.TrimPrefix(string(src.PathOriginal()), "/api")
	s.setRequestURI(dest, []byte(path))
	dest.SetQueryStringBytes(src.QueryString())
}
