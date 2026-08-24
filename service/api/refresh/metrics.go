package refresh

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	metricsNamespace = "gateway"
	metricsSubsystem = "refresh_coordinator"
)

// refreshesLedTotal covers all three invokers: tier-1 leader, tier-2 leader, and a
// waiter that degraded to its own refresh.
var refreshesLedTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsSubsystem,
	Name:      "led_total",
	Help:      "Token refreshes actually executed against the API.",
})

// refreshesCoalescedTotal is the payoff metric: coordination avoiding a call.
var refreshesCoalescedTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsSubsystem,
	Name:      "coalesced_total",
	Help:      "Callers coalesced onto another caller's refresh instead of performing their own.",
})

// refreshWaitTimeoutsTotal covers both tiers of waiter.
var refreshWaitTimeoutsTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsSubsystem,
	Name:      "wait_timeouts_total",
	Help:      "Waiters that exhausted their wait budget and degraded to their own refresh.",
})

// redisLockFailuresTotal spans lock acquisition, publish, and polling.
var redisLockFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsSubsystem,
	Name:      "redis_lock_failures_total",
	Help:      "Redis errors during lock/result operations that caused a fallback.",
})
