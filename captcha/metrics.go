package captcha

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	metricsNamespace = "gateway"
	metricsSubsystem = "captcha"

	// resultSolved marks a verification that passed (reCAPTCHA reported success).
	resultSolved = "solved"
	// resultMissed marks a request that reached the verify step but failed it: a
	// missing challenge token or a reCAPTCHA "success:false" (bad/low-quality token).
	resultMissed = "missed"
	// resultError marks a transport or response-parse failure talking to Google.
	resultError = "error"
	// resultDisabled marks the empty-secret short-circuit (captcha turned off) and the
	// CORS preflight bypass — no verification is performed.
	resultDisabled = "disabled"
)

// captchaVerificationsTotal counts every call to Verify, bucketed by outcome.
var captchaVerificationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsSubsystem,
	Name:      "verifications_total",
	Help:      "reCAPTCHA verifications by outcome: solved, missed, error, disabled.",
}, []string{"result"})

// captchaScore observes the reCAPTCHA v3 score whenever Google returns one.
var captchaScore = promauto.NewHistogram(prometheus.HistogramOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsSubsystem,
	Name:      "score",
	Help:      "reCAPTCHA v3 score returned by Google (0.0 bot .. 1.0 human).",
	Buckets:   []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
})

func observeCaptchaResult(result string) {
	captchaVerificationsTotal.WithLabelValues(result).Inc()
}

// observeCaptchaScore records a score only when Google actually returned one
// (score is absent on most failure responses, unmarshalling to 0).
func observeCaptchaScore(score float32) {
	if score > 0 {
		captchaScore.Observe(float64(score))
	}
}
