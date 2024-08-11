package retryhttp

import (
	"log"
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

	if attempts == 1 || err == nil || !strings.Contains(err.Error(), "broken pipe") {
		return err
	}

	log.Printf("retrying request due to an error [attempt %d] : %s", attempts, err.Error())

	return c.DoWithRetry(req, resp, attempts-1)
}

func (c *FastHttpRetryClient) WithRetryAttempts(attempts uint) Client {
	c.attempts = attempts
	return c
}
