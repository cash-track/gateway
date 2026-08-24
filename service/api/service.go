package api

import (
	"strings"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/http/retryhttp"
	"github.com/cash-track/gateway/router/csrf"
	"github.com/cash-track/gateway/service/api/refresh"
)

const (
	ServiceId         = "API"
	httpReadTimeout   = 5 * time.Second
	httpWriteTimeout  = 5 * time.Second
	httpRetryAttempts = uint(2)

	// refreshHttpTimeout bounds the single POST /auth/refresh call (see
	// doWithBreakerTimeout), tighter than httpReadTimeout+httpWriteTimeout since its
	// payload is small and fixed-shape — keeps RefreshLockTTL small enough that
	// waitBudget stays under the SPA's REQUEST_TIMEOUT_MS. See RefreshLockTTL.
	refreshHttpTimeout = 3 * time.Second

	// refreshLockMargin covers jitter, the coordinator's own Redis round trips, and GC
	// pauses on top of refreshHttpTimeout; too little and a second instance can acquire
	// the lock mid-refresh and duplicate it.
	refreshLockMargin = 5 * time.Second
)

// RefreshLockTTL sizes the Redis refresh lock (see refresh.RedisCoordinator) to safely
// cover one refreshToken call. Keeping it derived from refreshHttpTimeout, not the
// general forwarding timeouts, is what keeps the coordinator's waitBudget under the
// SPA's REQUEST_TIMEOUT_MS (frontend/src/api/client.ts) — otherwise the browser aborts
// before a legitimately slow leader responds.
func RefreshLockTTL() time.Duration {
	return refreshHttpTimeout + refreshLockMargin
}

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
	refresh refresh.Coordinator
}

func NewHttp(
	http retryhttp.Client,
	config config.Config,
	csrf csrf.CSRFSeeder,
	breaker *gobreaker.CircuitBreaker[struct{}],
	refresh refresh.Coordinator,
) *HttpService {
	http.WithReadTimeout(httpReadTimeout)
	http.WithWriteTimeout(httpWriteTimeout)
	http.WithRetryAttempts(httpRetryAttempts)

	return &HttpService{
		http:    http,
		config:  config,
		csrf:    csrf,
		breaker: breaker,
		refresh: refresh,
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
