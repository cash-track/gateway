package http

import (
	"time"

	"github.com/valyala/fasthttp"
)

const (
	ReadTimeout  = 5 * time.Second
	WriteTimeout = 5 * time.Second
	Concurrency  = 4096
)

type Client interface {
	Do(req *fasthttp.Request, resp *fasthttp.Response) error
	WithReadTimeout(timeout time.Duration) Client
	WithWriteTimeout(timeout time.Duration) Client
}

type FastHttpClient struct {
	*fasthttp.Client
}

func NewFastHttpClient() Client {
	return &FastHttpClient{
		Client: &fasthttp.Client{
			ReadTimeout:                   ReadTimeout,
			WriteTimeout:                  WriteTimeout,
			MaxIdleConnDuration:           time.Hour,
			NoDefaultUserAgentHeader:      true,
			DisableHeaderNamesNormalizing: true,
			DisablePathNormalizing:        true,
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
