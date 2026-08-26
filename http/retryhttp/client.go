package retryhttp

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"syscall"

	"github.com/cash-track/gateway/http"
	"github.com/valyala/fasthttp"
)

// defaultRetryAttempts gives one retry to consumers that never call WithRetryAttempts —
// jwks.HttpProvider is the only one. At 1 its hourly key refresh had a single attempt and
// died on the first stale socket.
const defaultRetryAttempts = 2

type Client interface {
	http.Client
	WithRetryAttempts(attempts uint) Client
	DoWithRetry(req *fasthttp.Request, resp *fasthttp.Response, attempts uint) error
}

type FastHttpRetryClient struct {
	http.Client

	attempts uint
}

func NewFastHttpRetryClient() Client {
	return &FastHttpRetryClient{
		Client:   http.NewFastHttpClient(),
		attempts: defaultRetryAttempts,
	}
}

func (c *FastHttpRetryClient) Do(req *fasthttp.Request, resp *fasthttp.Response) error {
	return c.DoWithRetry(req, resp, c.attempts)
}

func (c *FastHttpRetryClient) DoWithRetry(req *fasthttp.Request, resp *fasthttp.Response, attempts uint) error {
	err := c.Client.Do(req, resp)

	if attempts == 1 || err == nil || !isRetryableMethod(req) || !isRetryableTransportError(err) {
		return err
	}

	// Method and URI make the line correlatable: this layer has no request context, so
	// without them a retry warning cannot be tied to the error it preceded.
	slog.Warn("retrying request due to an error",
		"attempt", attempts,
		"method", string(req.Header.Method()),
		"uri", string(req.URI().Path()),
		"error", err,
	)

	return c.DoWithRetry(req, resp, attempts-1)
}

// isRetryableMethod reports whether req may be safely replayed after a transport failure.
// A dead connection cannot distinguish "never arrived" from "committed, then the socket
// died", so only the methods that are safe to deliver twice qualify. http.Client's RetryIf
// one layer down carries the matching gate.
func isRetryableMethod(req *fasthttp.Request) bool {
	return req.Header.IsGet() || req.Header.IsHead()
}

// isRetryableTransportError reports whether err means the connection failed rather than
// the backend answering. Those are the errors a fresh connection can recover from.
//
// fasthttp surfaces a dropped keep-alive socket as ErrConnectionClosed, not as a broken
// pipe; matching only the latter is what turned an API redeploy into a burst of 502s.
// Timeouts are deliberately absent: the request may well have been delivered, and a
// replay only doubles the latency budget.
func isRetryableTransportError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, fasthttp.ErrConnectionClosed) || errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	// Some paths lose the sentinel to fmt.Errorf without %w, so the text is the only
	// signal left.
	text := err.Error()

	return strings.Contains(text, "broken pipe") ||
		strings.Contains(text, "connection reset") ||
		strings.Contains(text, "closed connection")
}

func (c *FastHttpRetryClient) WithRetryAttempts(attempts uint) Client {
	c.attempts = attempts

	return c
}
