package retryhttp

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// retriesTotal counts transport-error retries across every consumer of this
// client (API forward path, captcha verify, JWKS refresh). A rising rate points
// at upstream connection churn (redeploys, keep-alive drops) before it turns
// into user-visible 502s.
var retriesTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "gateway",
	Subsystem: "upstream",
	Name:      "retries_total",
	Help:      "Outbound HTTP requests retried after a retryable transport error.",
})
