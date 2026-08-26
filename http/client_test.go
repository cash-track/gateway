package http

import (
	"bufio"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestNewFastHttpClient(t *testing.T) {
	client := NewFastHttpClient()
	assert.NotNil(t, client)

	fhc, ok := client.(*FastHttpClient)
	assert.True(t, ok)
	assert.Equal(t, 1, fhc.MaxIdemponentCallAttempts)
	assert.NotNil(t, fhc.RetryIf)
}

// The pool must not outlive the backend. At an hour, every API container replacement left
// the gateway holding dead sockets and answering 502 until they aged out.
func TestNewFastHttpClientIdleConnDuration(t *testing.T) {
	fhc := NewFastHttpClient().(*FastHttpClient)

	assert.Positive(t, fhc.MaxIdleConnDuration)
	assert.LessOrEqual(t, fhc.MaxIdleConnDuration, 30*time.Second,
		"idle sockets must be reaped soon enough that a container swap does not strand them")
}

// Tests the RetryIf predicate in isolation; it cannot catch MaxIdemponentCallAttempts
// being raised above 1 (see io.EOF in client.go). That is covered end-to-end by
// TestDoNotRetriedAtThisLayerRegardlessOfMethod below.
func TestNewFastHttpClientRetryIf(t *testing.T) {
	client := NewFastHttpClient().(*FastHttpClient)

	tests := []struct {
		method string
		want   bool
	}{
		{fasthttp.MethodGet, true},
		{fasthttp.MethodHead, true},
		{fasthttp.MethodPost, false},
		{fasthttp.MethodPut, false},
		{fasthttp.MethodPatch, false},
		{fasthttp.MethodDelete, false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			req := &fasthttp.Request{}
			req.Header.SetMethod(tt.method)

			assert.Equal(t, tt.want, client.RetryIf(req))
		})
	}
}

// readFullRequest reads headers plus any Content-Length body, reporting whether a full
// request arrived.
func readFullRequest(c net.Conn) bool {
	br := bufio.NewReader(c)
	contentLength := 0

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return false
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			for _, ch := range strings.TrimSpace(line[len("content-length:"):]) {
				if ch >= '0' && ch <= '9' {
					contentLength = contentLength*10 + int(ch-'0')
				}
			}
		}
		if line == "\r\n" {
			break
		}
	}

	if contentLength == 0 {
		return true
	}

	body := make([]byte, contentLength)
	for read := 0; read < contentLength; {
		n, err := br.Read(body[read:])
		if err != nil {
			return false
		}
		read += n
	}

	return true
}

// silentCloseBackend accepts one connection, fully reads the request, then closes without
// responding — the io.EOF case. Returns the listener and a delivered-request counter.
func silentCloseBackend(t *testing.T) (net.Listener, *int64) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	var delivered int64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				if readFullRequest(c) {
					atomic.AddInt64(&delivered, 1)
				}
			}(c)
		}
	}()

	return ln, &delivered
}

// Regression test for MaxIdemponentCallAttempts: drives the real client against a backend
// that reads each request then closes silently. Every method must be delivered exactly
// once; raising the attempt count above 1 makes this observe delivered > 1.
func TestDoNotRetriedAtThisLayerRegardlessOfMethod(t *testing.T) {
	methods := []string{
		fasthttp.MethodGet,
		fasthttp.MethodHead,
		fasthttp.MethodPost,
		fasthttp.MethodPut,
		fasthttp.MethodPatch,
		fasthttp.MethodDelete,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			ln, delivered := silentCloseBackend(t)
			defer func() { _ = ln.Close() }()

			client := NewFastHttpClient()
			client.WithReadTimeout(2 * time.Second)
			client.WithWriteTimeout(2 * time.Second)

			req := fasthttp.AcquireRequest()
			defer fasthttp.ReleaseRequest(req)
			resp := fasthttp.AcquireResponse()
			defer fasthttp.ReleaseResponse(resp)

			req.Header.SetMethod(method)
			req.SetRequestURI("http://" + ln.Addr().String() + "/v1/wallets/1/charges")
			req.Header.SetContentType("application/json")
			req.SetBody([]byte(`{"type":"-","amount":100,"title":"coffee"}`))

			err := client.Do(req, resp)

			assert.Error(t, err)
			assert.EqualValues(t, 1, atomic.LoadInt64(delivered),
				"%s must be delivered to the backend exactly once at this layer, "+
					"regardless of RetryIf — MaxIdemponentCallAttempts must stay at 1", method)
		})
	}
}

func TestWithReadTimeout(t *testing.T) {
	client := FastHttpClient{
		Client: &fasthttp.Client{},
	}
	client.WithReadTimeout(1 * time.Second)

	assert.Equal(t, 1*time.Second, client.ReadTimeout)
}

func TestWithWriteTimeout(t *testing.T) {
	client := FastHttpClient{
		Client: &fasthttp.Client{},
	}
	client.WithWriteTimeout(1 * time.Second)

	assert.Equal(t, 1*time.Second, client.WriteTimeout)
}
