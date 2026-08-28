package api

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/valyala/fasthttp"
)

const (
	metricsUpstreamSubsys     = "upstream"
	metricsTokenRefreshSubsys = "token_refresh"

	// upstreamStatusError labels a forwarded call that failed before any HTTP
	// response was received (transport error or open circuit breaker).
	upstreamStatusError = "error"

	// tokenRefreshSuccess: a fresh access token was obtained and the request retried.
	tokenRefreshSuccess = "success"
	// tokenRefreshFailed: the refresh token was genuinely rejected; the session is
	// cleared and the user must log in again.
	tokenRefreshFailed = "failed"
	// tokenRefreshSessionPreserved: a transient API failure during refresh; cookies are
	// kept and a 503 is returned so the client can retry.
	tokenRefreshSessionPreserved = "session_preserved"
)

// upstreamRequestDuration measures only the forwarded round trip to the PHP API,
// as distinct from the total gateway handler latency tracked by the http subsystem.
var upstreamRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsUpstreamSubsys,
	Name:      "request_duration_seconds",
	Help:      "Duration of the forwarded request to the PHP API (upstream round trip time).",
	Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
}, []string{"method"})

// upstreamRequestsTotal counts forwarded requests to the PHP API by method and
// response status class (2xx/3xx/4xx/5xx/other).
var upstreamRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsUpstreamSubsys,
	Name:      "requests_total",
	Help:      "Forwarded requests to the PHP API by method and response status class.",
}, []string{"method", "status"})

// upstreamRequestsInFlight tracks forwarded requests currently awaiting a PHP API response.
var upstreamRequestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsUpstreamSubsys,
	Name:      "requests_in_flight",
	Help:      "Forwarded requests to the PHP API currently awaiting a response.",
})

// tokenRefreshTotal counts auto-refresh attempts on the forward path by outcome.
var tokenRefreshTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsTokenRefreshSubsys,
	Name:      "total",
	Help:      "Automatic token-refresh attempts by outcome: success, failed, session_preserved.",
}, []string{"result"})

// tokenRefreshDuration measures the wall time of the coordinated refresh step
// (includes any time a caller spent coalesced onto another caller's refresh).
var tokenRefreshDuration = promauto.NewHistogram(prometheus.HistogramOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsTokenRefreshSubsys,
	Name:      "duration_seconds",
	Help:      "Wall time of the coordinated token-refresh step on the forward path.",
	Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
})

// withInFlight runs fn while the in-flight gauge is incremented, guaranteeing the
// matching decrement even if fn panics — mirrors client_golang's
// InstrumentHandlerInFlight helper.
func withInFlight(g prometheus.Gauge, fn func() error) error {
	g.Inc()
	defer g.Dec()

	return fn()
}

// statusClass buckets an HTTP status code into a bounded label value.
func statusClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500 && code < 600:
		return "5xx"
	default:
		return "other"
	}
}

// observeUpstream records the duration and status class of one forwarded API call.
func observeUpstream(method string, statusCode int, seconds float64) {
	upstreamRequestDuration.WithLabelValues(method).Observe(seconds)
	upstreamRequestsTotal.WithLabelValues(method, statusClass(statusCode)).Inc()
}

var knownUpstreamMethods = map[string]bool{
	fasthttp.MethodGet:     true,
	fasthttp.MethodHead:    true,
	fasthttp.MethodPost:    true,
	fasthttp.MethodPut:     true,
	fasthttp.MethodPatch:   true,
	fasthttp.MethodDelete:  true,
	fasthttp.MethodOptions: true,
}

// upstreamMethod bounds the method label to a fixed set, mapping anything
// unexpected to "other" so a malformed request can't inflate cardinality.
func upstreamMethod(method string) string {
	if knownUpstreamMethods[method] {
		return method
	}

	return "other"
}
