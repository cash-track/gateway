package api

import (
	"errors"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sony/gobreaker/v2"
	"github.com/valyala/fasthttp"
)

const (
	breakerMaxRequests = 5
	// Zero keeps failures counted across the whole closed state. A windowed reset would
	// clear the count between slow failures, so a hanging backend would never trip.
	breakerInterval         = time.Duration(0)
	breakerTimeout          = 30 * time.Second
	breakerFailureThreshold = 10
	metricsNamespace        = "gateway"
	metricsApiBreakerSubsys = "api_breaker"
)

// RetryAfterSeconds is the Retry-After hint sent with 503s while the breaker is open.
const RetryAfterSeconds = int(breakerTimeout / time.Second)

// ErrCircuitOpen lets callers tell a tripped breaker from a plain transport error.
var ErrCircuitOpen = errors.New("circuit breaker is open")

var breakerRejectedTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsApiBreakerSubsys,
	Name:      "rejected_total",
	Help:      "Requests rejected without hitting the API because the circuit breaker was open or half-open and full.",
})

// NewBreaker builds the API circuit breaker. Call once per process and share the
// instance across requests.
func NewBreaker() *gobreaker.CircuitBreaker[struct{}] {
	return gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name:        ServiceId,
		MaxRequests: breakerMaxRequests,
		Interval:    breakerInterval,
		Timeout:     breakerTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > breakerFailureThreshold
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Printf("[%s] circuit breaker state change: %s -> %s", name, from, to)
		},
	})
}

// RegisterBreakerMetrics exposes the breaker state on the default Prometheus registry.
// Call once — a second call panics on duplicate registration.
func RegisterBreakerMetrics(breaker *gobreaker.CircuitBreaker[struct{}]) {
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsApiBreakerSubsys,
		Name:      "state",
		Help:      "API circuit breaker state: 0=closed, 1=half-open, 2=open.",
	}, func() float64 {
		return float64(breaker.State())
	})
}

// doWithBreaker runs the API call through the breaker. Transport errors pass through
// unwrapped; a rejected call yields ErrCircuitOpen.
func (s *HttpService) doWithBreaker(req *fasthttp.Request, resp *fasthttp.Response) error {
	_, err := s.breaker.Execute(func() (struct{}, error) {
		return struct{}{}, s.http.Do(req, resp)
	})

	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		breakerRejectedTotal.Inc()

		return ErrCircuitOpen
	}

	return err
}
