package api

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/valyala/fasthttp"
)

const (
	metricsNamespace  = "gateway"
	metricsAuthSubsys = "auth"

	authActionLogin       = "login"
	authActionRegister    = "register"
	authActionPasskey     = "passkey"
	authActionPasskeyInit = "passkey_init"
	authActionGoogle      = "google"
	authActionLogout      = "logout"

	authResultSuccess = "success"
	authResultFailure = "failure"

	passkeyInitPathSuffix = "/passkey/init"
)

// authAttemptsTotal counts edge auth outcomes the gateway can observe from the
// response it produced (cookies set / backend status), bucketed by action.
var authAttemptsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsAuthSubsys,
	Name:      "attempts_total",
	Help:      "Edge auth attempts by action (login, register, passkey, passkey_init, google, logout) and result.",
}, []string{"action", "result"})

func recordAuthAttempt(action, result string) {
	authAttemptsTotal.WithLabelValues(action, result).Inc()
}

// authResultFromStatus maps the response status the handler ended up with to a
// bounded success/failure label. A 2xx means the gateway completed the flow
// (auth cookies written, or logout forwarded); anything else is a failure.
func authResultFromStatus(status int) string {
	if status >= fasthttp.StatusOK && status < fasthttp.StatusMultipleChoices {
		return authResultSuccess
	}

	return authResultFailure
}

// authActionFromPath derives the action label from the request path. The auth-set
// routes all share one handler, so the path is the only discriminator.
func authActionFromPath(path []byte) string {
	p := string(path)

	switch {
	case strings.HasSuffix(p, "/passkey"):
		return authActionPasskey
	case strings.HasSuffix(p, "/register"):
		return authActionRegister
	case strings.HasSuffix(p, "/provider/google"):
		return authActionGoogle
	default:
		return authActionLogin
	}
}

func isPasskeyInitPath(path []byte) bool {
	return strings.HasSuffix(string(path), passkeyInitPathSuffix)
}
