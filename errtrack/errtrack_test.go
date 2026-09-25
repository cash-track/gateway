package errtrack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

const validTraceId = "0af7651916cd43dd8448eb211c80319c"

func newTestHandler(tempoURL string) (*Handler, *[]*sentry.Event, *bytes.Buffer) {
	var events []*sentry.Event
	out := &bytes.Buffer{}
	h := NewHandler(slog.NewJSONHandler(out, nil), tempoURL)
	h.capture = func(e *sentry.Event) *sentry.EventID {
		events = append(events, e)
		return nil
	}

	return h, &events, out
}

func TestInitEmptyDsn(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	assert.NoError(t, Init("v1.0.0"))
}

func TestInitInvalidDsn(t *testing.T) {
	t.Setenv("SENTRY_DSN", "not a dsn")
	assert.Error(t, Init("v1.0.0"))
}

func TestHandlerCapturesErrorRecord(t *testing.T) {
	h, events, out := newTestHandler("http://grafana/{trace_id}")
	log := slog.New(h).With("component", "gateway")

	log.Error("forward request failed", "trace_id", validTraceId, "error", errors.New("dial tcp: refused"))

	assert.Len(t, *events, 1)
	e := (*events)[0]
	assert.Equal(t, sentry.LevelError, e.Level)
	assert.Equal(t, "forward request failed: dial tcp: refused", e.Message)
	assert.Equal(t, []string{"forward request failed"}, e.Fingerprint)
	assert.Equal(t, "gateway", e.Tags["service"])
	assert.Equal(t, validTraceId, e.Tags["trace_id"])
	assert.Equal(t, sentry.Context{"trace_id": validTraceId, "url": "http://grafana/" + validTraceId}, e.Contexts["tempo"])
	assert.Equal(t, "gateway", e.Contexts["extra"]["component"])
	assert.Contains(t, out.String(), "forward request failed")
	assert.Len(t, e.Exception, 1)
	assert.Equal(t, "forward request failed", e.Exception[0].Type)
	assert.Equal(t, "dial tcp: refused", e.Exception[0].Value)
	assert.NotEmpty(t, e.Exception[0].Stacktrace.Frames)
	assert.Equal(t, "*errors.errorString", e.Contexts["extra"]["error_type"])
}

func TestHandlerErrorTypeUnwrapsToRoot(t *testing.T) {
	h, events, _ := newTestHandler("")

	slog.New(h).Error("boom", "error", fmt.Errorf("outer: %w", &net.OpError{Op: "dial", Err: errors.New("refused")}))

	assert.Equal(t, "*errors.errorString", (*events)[0].Contexts["extra"]["error_type"])

	slog.New(h).Error("boom", "error", fmt.Errorf("outer: %w", &net.DNSError{Err: "nx"}))

	assert.Equal(t, "*net.DNSError", (*events)[1].Contexts["extra"]["error_type"])
}

func TestHandlerNonErrorValueHasNoErrorType(t *testing.T) {
	h, events, _ := newTestHandler("")

	slog.New(h).Error("boom", "error", "plain string")

	assert.Equal(t, "plain string", (*events)[0].Exception[0].Value)
	assert.NotContains(t, (*events)[0].Contexts["extra"], "error_type")
}

func TestHandlerExcludesClientIpFromExtra(t *testing.T) {
	h, events, _ := newTestHandler("")

	slog.New(h).Error("captcha verify request failed", "client_ip", "203.0.113.5", "provider", "recaptcha")

	assert.Len(t, *events, 1)
	extra := (*events)[0].Contexts["extra"]
	assert.NotContains(t, extra, "client_ip")
	assert.Equal(t, "recaptcha", extra["provider"])
}

func TestHandlerSkipsBelowError(t *testing.T) {
	h, events, out := newTestHandler("")

	slog.New(h).Warn("circuit open", "trace_id", validTraceId)

	assert.Empty(t, *events)
	assert.Contains(t, out.String(), "circuit open")
}

func TestHandlerSkipsInvalidTraceId(t *testing.T) {
	for _, id := range []string{"", strings.Repeat("0", 32), "zz"} {
		h, events, _ := newTestHandler("http://grafana/{trace_id}")

		slog.New(h).Error("boom", "trace_id", id)

		assert.Len(t, *events, 1)
		assert.NotContains(t, (*events)[0].Tags, "trace_id")
		assert.NotContains(t, (*events)[0].Contexts, "tempo")
	}
}

func TestHandlerNoTempoContextWithoutTemplate(t *testing.T) {
	h, events, _ := newTestHandler("")

	slog.New(h).Error("boom", "trace_id", validTraceId)

	assert.Equal(t, validTraceId, (*events)[0].Tags["trace_id"])
	assert.NotContains(t, (*events)[0].Contexts, "tempo")
}

func TestHandlerWithGroupAndEnabled(t *testing.T) {
	h, events, out := newTestHandler("")

	slog.New(h.WithGroup("g")).Error("boom", "k", "v")

	assert.Len(t, *events, 1)
	assert.Contains(t, out.String(), `"g":{"k":"v"}`)
	assert.False(t, h.Enabled(context.Background(), slog.LevelDebug))
}

func TestRecoverHandlerReportsAndRepanics(t *testing.T) {
	h, events, _ := newTestHandler("")
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })

	handler := RecoverHandler(func(*fasthttp.RequestCtx) { panic("kaboom") })

	assert.PanicsWithValue(t, "kaboom", func() { handler(&fasthttp.RequestCtx{}) })
	assert.Len(t, *events, 1)
	assert.Equal(t, "panic while handling request", (*events)[0].Fingerprint[0])
	assert.Contains(t, (*events)[0].Message, "kaboom")
	assert.NotEmpty(t, (*events)[0].Exception[0].Stacktrace.Frames)
}

func TestHandlerStackEndsAtCallSite(t *testing.T) {
	h, events, _ := newTestHandler("")

	slog.New(h).Error("boom")

	frames := (*events)[0].Exception[0].Stacktrace.Frames
	assert.Equal(t, "TestHandlerStackEndsAtCallSite", frames[len(frames)-1].Function)
	assert.True(t, frames[len(frames)-1].InApp)
}

func panickingFunc() { panic("kaboom") }

func TestRecoverHandlerStackEndsAtPanic(t *testing.T) {
	h, events, _ := newTestHandler("")
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })

	handler := RecoverHandler(func(*fasthttp.RequestCtx) { panickingFunc() })
	assert.Panics(t, func() { handler(&fasthttp.RequestCtx{}) })

	frames := (*events)[0].Exception[0].Stacktrace.Frames
	assert.Equal(t, "panickingFunc", frames[len(frames)-1].Function)
	assert.True(t, frames[len(frames)-1].InApp)
}

func TestFlush(t *testing.T) {
	assert.NotPanics(t, Flush)
}

func TestRecoverHandlerPassesThrough(t *testing.T) {
	called := false

	RecoverHandler(func(*fasthttp.RequestCtx) { called = true })(&fasthttp.RequestCtx{})

	assert.True(t, called)
}
