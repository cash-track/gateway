package retryhttp

import (
	"log/slog"
	"strings"

	"github.com/cash-track/gateway/http"
	"github.com/valyala/fasthttp"
)

const defaultRetryAttempts = 1

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

	// A broken pipe means the write failed, not that the backend never read and committed
	// the request. Only GET/HEAD are safe to replay blind; http.Client's RetryIf one layer
	// down carries the matching gate.
	if attempts == 1 || err == nil || !isRetryableMethod(req) || !strings.Contains(err.Error(), "broken pipe") {
		return err
	}

	slog.Warn("retrying request due to an error", "attempt", attempts, "error", err)

	return c.DoWithRetry(req, resp, attempts-1)
}

// isRetryableMethod reports whether req may be safely replayed after a write failure.
func isRetryableMethod(req *fasthttp.Request) bool {
	return req.Header.IsGet() || req.Header.IsHead()
}

func (c *FastHttpRetryClient) WithRetryAttempts(attempts uint) Client {
	c.attempts = attempts

	return c
}
