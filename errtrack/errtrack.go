// Package errtrack reports gateway errors to Sentry. Errors only: Tempo owns tracing.
package errtrack

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/cash-track/gateway/traces"
)

const (
	flushTimeout   = 2 * time.Second
	errtrackModule = "github.com/cash-track/gateway/errtrack"
)

// ignoredErrors are regexps matched against the event message; add expected noise here.
var ignoredErrors = []string{
	"context canceled", // client went away
}

// Init configures the global Sentry client from SENTRY_DSN / SENTRY_ENVIRONMENT; empty DSN is a no-op.
func Init(release string) error {
	if release != "" {
		release = "gateway@" + release
	}

	return sentry.Init(sentry.ClientOptions{
		Release:       release,
		EnableTracing: false,
		IgnoreErrors:  ignoredErrors,
	})
}

// Handler forwards every record to next and reports ERROR+ records to Sentry.
type Handler struct {
	next     slog.Handler
	attrs    []slog.Attr
	tempoURL string
	capture  func(*sentry.Event) *sentry.EventID
}

func NewHandler(next slog.Handler, tempoURL string) *Handler {
	return &Handler{next: next, tempoURL: tempoURL, capture: sentry.CaptureEvent}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		h.capture(h.event(r))
	}

	return h.next.Handle(ctx, r)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.next = h.next.WithAttrs(attrs)
	c.attrs = append(slices.Clone(h.attrs), attrs...)

	return &c
}

func (h *Handler) WithGroup(name string) slog.Handler {
	c := *h
	c.next = h.next.WithGroup(name)

	return &c
}

func (h *Handler) event(r slog.Record) *sentry.Event {
	e := sentry.NewEvent()
	e.Level = sentry.LevelError
	e.Message = r.Message
	e.Fingerprint = []string{r.Message}
	e.Tags["service"] = "gateway"

	// sentry-go v0.49.0 dropped the top-level Extra field; a named context is the replacement.
	extra := sentry.Context{}
	exc := sentry.Exception{Type: r.Message, Stacktrace: callerStack()}

	add := func(a slog.Attr) bool {
		switch {
		case a.Key == "error":
			e.Message += ": " + a.Value.String()
			exc.Value = a.Value.String()
			if err, ok := a.Value.Any().(error); ok {
				extra["error_type"] = rootType(err)
			}
		case a.Key == "trace_id":
			h.tagTrace(e, a.Value.String())
		}
		// client_ip is PII; Loki keeps the full log line, Sentry does not need it.
		if a.Key != "client_ip" {
			extra[a.Key] = a.Value.String()
		}

		return true
	}

	for _, a := range h.attrs {
		add(a)
	}
	r.Attrs(add)

	// Exception carries the call-site stack (for panics: the panicking frames).
	e.Exception = []sentry.Exception{exc}

	if len(extra) > 0 {
		e.Contexts["extra"] = extra
	}

	return e
}

// callerStack drops the logging frames (slog, Handle, event) so the innermost frame is the
// slog call site. For panics sentry already cuts at runtime.gopanic, leaving the panicking frame.
func callerStack() *sentry.Stacktrace {
	st := sentry.NewStacktrace()
	f := st.Frames
	for len(f) > 0 && isLoggingFrame(f[len(f)-1]) {
		f = f[:len(f)-1]
	}
	st.Frames = f

	return st
}

var loggingFuncs = map[string]bool{"(*Handler).Handle": true, "(*Handler).event": true, "callerStack": true}

func isLoggingFrame(f sentry.Frame) bool {
	return f.Module == "log/slog" || f.Module == errtrackModule && loggingFuncs[f.Function]
}

// rootType names the innermost wrapped error type, e.g. *net.OpError.
func rootType(err error) string {
	for u := errors.Unwrap(err); u != nil; u = errors.Unwrap(err) {
		err = u
	}

	return fmt.Sprintf("%T", err)
}

func (h *Handler) tagTrace(e *sentry.Event, id string) {
	if tid, err := trace.TraceIDFromHex(id); err != nil || !tid.IsValid() {
		return
	}

	e.Tags["trace_id"] = id
	if h.tempoURL != "" {
		e.Contexts["tempo"] = sentry.Context{"trace_id": id, "url": strings.ReplaceAll(h.tempoURL, "{trace_id}", id)}
	}
}

// Flush blocks until buffered events are sent or flushTimeout elapses.
func Flush() {
	sentry.Flush(flushTimeout)
}

// RecoverHandler reports a panic with its trace id, then re-panics so crash behaviour is unchanged.
func RecoverHandler(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic while handling request", "trace_id", traces.FindTraceId(ctx), "error", fmt.Sprint(rec))
				Flush()
				panic(rec)
			}
		}()

		next(ctx)
	}
}
