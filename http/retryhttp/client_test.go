package retryhttp

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/http"
	httpmock "github.com/cash-track/gateway/mocks/http"
)

func TestNewFastHttpRetryClient(t *testing.T) {
	client := NewFastHttpRetryClient()
	assert.NotNil(t, client)
}

func TestDoWithRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := httpmock.NewClientMock(ctrl)
	c.EXPECT().Do(gomock.Any(), gomock.Any()).Times(2).Return(fmt.Errorf("unknown error: broken pipe or closed connection"))

	client := FastHttpRetryClient{
		Client: c,
	}

	client.WithRetryAttempts(2)
	err := client.Do(&fasthttp.Request{}, &fasthttp.Response{})

	assert.Error(t, err)
}

func TestDoWithRetryMutatingMethodNotRetried(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := httpmock.NewClientMock(ctrl)
	c.EXPECT().Do(gomock.Any(), gomock.Any()).Times(1).Return(fmt.Errorf("unknown error: broken pipe or closed connection"))

	client := FastHttpRetryClient{
		Client: c,
	}
	client.WithRetryAttempts(2)

	req := &fasthttp.Request{}
	req.Header.SetMethod(fasthttp.MethodPost)

	err := client.Do(req, &fasthttp.Response{})

	assert.Error(t, err)
}

func TestIsRetryableMethod(t *testing.T) {
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

			assert.Equal(t, tt.want, isRetryableMethod(req))
		})
	}
}

func TestWithRetryAttempts(t *testing.T) {
	client := FastHttpRetryClient{
		Client: &http.FastHttpClient{},
	}
	client.WithRetryAttempts(3)

	assert.Equal(t, uint(3), client.attempts)
}

// Regression coverage for the no-retry-on-mutating-methods gates. These drive the real
// client stack against a listener that behaves like a worker which commits the write then
// dies: it fully reads the request before misbehaving. A regression in either gate shows
// up as the same mutating request being delivered more than once.

// readFullRequestLine reads headers plus any Content-Length body, reporting whether a full
// request arrived.
func readFullRequestLine(c net.Conn) bool {
	br := bufio.NewReader(c)
	contentLength := 0

	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return false
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			fmt.Sscanf(strings.TrimSpace(line[len("content-length:"):]), "%d", &contentLength)
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

// misbehavingBackend accepts connections, fully reads and counts each request, then runs
// behave on the raw connection. behave gets a stop channel closed at t.Cleanup so a
// blocking behaviour can exit early instead of outliving the test.
func misbehavingBackend(t *testing.T, delivered *int64, behave func(c net.Conn, stop <-chan struct{})) net.Listener {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	conns := make(map[net.Conn]struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			mu.Lock()
			conns[c] = struct{}{}
			mu.Unlock()

			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer func() {
					mu.Lock()
					delete(conns, c)
					mu.Unlock()
					_ = c.Close()
				}()
				if readFullRequestLine(c) {
					atomic.AddInt64(delivered, 1)
					behave(c, stop)
				}
			}(c)
		}
	}()

	t.Cleanup(func() {
		_ = ln.Close()
		close(stop)

		mu.Lock()
		for c := range conns {
			_ = c.Close()
		}
		mu.Unlock()

		wg.Wait()
	})

	return ln
}

// The io.EOF replay: the backend reads the POST body, so it could have committed the
// write, then closes without answering.
func TestPOSTDeliveredOnceOnSilentClose(t *testing.T) {
	var delivered int64
	ln := misbehavingBackend(t, &delivered, func(c net.Conn, stop <-chan struct{}) { /* close with no response */ })

	c := NewFastHttpRetryClient()
	c.WithReadTimeout(2 * time.Second)
	c.WithWriteTimeout(2 * time.Second)
	c.WithRetryAttempts(2) // production value (service/api.httpRetryAttempts)

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.Header.SetMethod(fasthttp.MethodPost)
	req.SetRequestURI("http://" + ln.Addr().String() + "/v1/wallets/1/charges")
	req.Header.SetContentType("application/json")
	req.SetBody([]byte(`{"type":"-","amount":100,"title":"coffee"}`))

	err := c.Do(req, resp)

	assert.Error(t, err)
	assert.EqualValues(t, 1, atomic.LoadInt64(&delivered),
		"POST must be delivered to the backend exactly once, not replayed on io.EOF")
}

// The read-timeout replay: the backend reads the PUT body and never answers. It outlasts
// the client's 200ms read timeout, then bails out via stop at cleanup.
func TestPUTDeliveredOnceOnReadTimeout(t *testing.T) {
	var delivered int64
	ln := misbehavingBackend(t, &delivered, func(c net.Conn, stop <-chan struct{}) {
		select {
		case <-time.After(3 * time.Second):
		case <-stop:
		}
	})

	c := NewFastHttpRetryClient()
	c.WithReadTimeout(200 * time.Millisecond)
	c.WithWriteTimeout(2 * time.Second)
	c.WithRetryAttempts(2)

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.Header.SetMethod(fasthttp.MethodPut)
	req.SetRequestURI("http://" + ln.Addr().String() + "/v1/profile/password")
	req.Header.SetContentType("application/json")
	req.SetBody([]byte(`{"password":"old","newPassword":"new"}`))

	err := c.Do(req, resp)

	assert.Error(t, err)
	assert.EqualValues(t, 1, atomic.LoadInt64(&delivered),
		"PUT must be delivered to the backend exactly once, not replayed on a read timeout")
}

// The third mutating method the audit measured being replayed.
func TestDELETEDeliveredOnceOnSilentClose(t *testing.T) {
	var delivered int64
	ln := misbehavingBackend(t, &delivered, func(c net.Conn, stop <-chan struct{}) { /* close with no response */ })

	c := NewFastHttpRetryClient()
	c.WithReadTimeout(2 * time.Second)
	c.WithRetryAttempts(2)

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.Header.SetMethod(fasthttp.MethodDelete)
	req.SetRequestURI("http://" + ln.Addr().String() + "/v1/wallets/1/charges/abc")

	err := c.Do(req, resp)

	assert.Error(t, err)
	assert.EqualValues(t, 1, atomic.LoadInt64(&delivered),
		"DELETE must be delivered to the backend exactly once, not replayed on io.EOF")
}
