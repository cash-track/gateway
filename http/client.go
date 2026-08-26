package http

import (
	"time"

	"github.com/valyala/fasthttp"
)

const (
	ReadTimeout  = 5 * time.Second
	WriteTimeout = 5 * time.Second
	Concurrency  = 4096

	// IdleConnDuration must stay well under the time a backend container survives a
	// redeploy, so a stale socket is reaped rather than handed to the next request.
	IdleConnDuration = 10 * time.Second
)

type Client interface {
	Do(req *fasthttp.Request, resp *fasthttp.Response) error
	// DoTimeout bounds one call by a single deadline, overriding ReadTimeout/WriteTimeout
	// for that call only — doesn't mutate client state, so it's safe alongside Do.
	DoTimeout(req *fasthttp.Request, resp *fasthttp.Response, timeout time.Duration) error
	WithReadTimeout(timeout time.Duration) Client
	WithWriteTimeout(timeout time.Duration) Client
}

type FastHttpClient struct {
	*fasthttp.Client
}

func NewFastHttpClient() Client {
	return &FastHttpClient{
		Client: &fasthttp.Client{
			ReadTimeout:  ReadTimeout,
			WriteTimeout: WriteTimeout,
			// Bounds how long a pooled socket may sit idle. An hour let the pool outlive
			// the backend: every API container replacement stranded connections the peer
			// had already dropped, and the first request on each answered 502.
			MaxIdleConnDuration:           IdleConnDuration,
			NoDefaultUserAgentHeader:      true,
			DisableHeaderNamesNormalizing: true,
			DisablePathNormalizing:        true,
			// No retries at this layer. fasthttp's defaults (5 attempts, PUT counted as
			// idempotent) blindly replay a mutating request whose write failed after the
			// backend already committed it.
			//
			// Do not raise above 1: fasthttp retries any method on io.EOF regardless of
			// RetryIf, so only this value closes that hole. GET/HEAD lose retries here as
			// a result; retryhttp.FastHttpRetryClient owns them instead, where the method
			// gate and the error classification are both explicit. Pinned by
			// TestDoNotRetriedAtThisLayerRegardlessOfMethod.
			MaxIdemponentCallAttempts: 1,
			// Unreachable at MaxIdemponentCallAttempts = 1; kept as a statement of intent.
			// PUT is excluded despite being RFC-idempotent: the gateway's PUT routes are
			// read-modify-write, and a write error can't distinguish "never arrived" from
			// "committed, then the connection died".
			RetryIf: func(req *fasthttp.Request) bool {
				return req.Header.IsGet() || req.Header.IsHead()
			},
			Dial: (&fasthttp.TCPDialer{
				Concurrency:      Concurrency,
				DNSCacheDuration: time.Hour,
			}).Dial,
		},
	}
}

func (c *FastHttpClient) WithReadTimeout(timeout time.Duration) Client {
	c.ReadTimeout = timeout

	return c
}

func (c *FastHttpClient) WithWriteTimeout(timeout time.Duration) Client {
	c.WriteTimeout = timeout

	return c
}
