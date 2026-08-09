package logger

import (
	"bytes"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
)

// setTestLogger redirects slog.Default() to a buffer, restored on cleanup.
func setTestLogger(t *testing.T) *bytes.Buffer {
	t.Helper()

	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))

	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	return &output
}

func TestDebugRequest(t *testing.T) {
	output := setTestLogger(t)

	config.Global.DebugHttp = false

	req := fasthttp.Request{}
	req.Header.Set("Host", "127.0.0.1")

	DebugRequest(&req, "API")

	logs := output.String()

	assert.NotContains(t, logs, "debug request")
	assert.NotContains(t, logs, "127.0.0.1")

	config.Global.DebugHttp = true

	DebugRequest(&req, "API")

	logs = output.String()

	assert.Contains(t, logs, "debug request")
	assert.Contains(t, logs, `"service":"API"`)
	assert.Contains(t, logs, "127.0.0.1")
}

func TestDebugResponse(t *testing.T) {
	output := setTestLogger(t)

	config.Global.DebugHttp = false

	resp := fasthttp.Response{}
	resp.Header.Set("Host", "127.0.0.1")

	DebugResponse(&resp, "API")

	logs := output.String()

	assert.NotContains(t, logs, "debug response")
	assert.NotContains(t, logs, "127.0.0.1")

	config.Global.DebugHttp = true

	DebugResponse(&resp, "API")

	logs = output.String()

	assert.Contains(t, logs, "debug response")
	assert.Contains(t, logs, `"service":"API"`)
	assert.Contains(t, logs, "127.0.0.1")
}

func TestDebugHandler(t *testing.T) {
	output := setTestLogger(t)
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Host", "127.0.0.1")
	ctx.Response.Header.Set("Host", "127.0.0.2")

	h := DebugHandler(func(ctx *fasthttp.RequestCtx) {})
	h(&ctx)

	logs := output.String()

	assert.Contains(t, logs, "debug request")
	assert.Contains(t, logs, "127.0.0.1")
	assert.Contains(t, logs, "debug response")
	assert.Contains(t, logs, "127.0.0.2")
}

func TestDebugHandlerIgnorePath(t *testing.T) {
	output := setTestLogger(t)
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.URI().SetPath("/live")
	ctx.Request.Header.Set("Host", "127.0.0.1")
	ctx.Response.Header.Set("Host", "127.0.0.2")

	h := DebugHandler(func(ctx *fasthttp.RequestCtx) {})
	h(&ctx)

	logs := output.String()

	assert.NotContains(t, logs, "debug request")
	assert.NotContains(t, logs, "127.0.0.1")
	assert.NotContains(t, logs, "127.0.0.2")
}

func TestFullForwarded(t *testing.T) {
	output := setTestLogger(t)

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetRequestURI("/test?one=two%203")
	ctx.Request.SetBody([]byte("123"))

	resp := fasthttp.Response{}
	resp.SetStatusCode(fasthttp.StatusBadRequest)
	resp.SetBody([]byte("4567"))

	FullForwarded(&ctx, &resp, "API", 42*time.Millisecond)

	logs := output.String()

	assert.Contains(t, logs, `"msg":"request forwarded"`)
	assert.Contains(t, logs, `"level":"INFO"`)
	assert.Contains(t, logs, `"client_ip":"10.0.0.1"`)
	assert.Contains(t, logs, `"method":"POST"`)
	assert.Contains(t, logs, `"path":"/test"`)
	assert.Contains(t, logs, `"status":400`)
	assert.Contains(t, logs, `"duration_ms":42`)
	assert.Contains(t, logs, `"service":"API"`)
}

func TestFullForwardedLogsAtWarnOn5xx(t *testing.T) {
	output := setTestLogger(t)

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/test")

	resp := fasthttp.Response{}
	resp.SetStatusCode(fasthttp.StatusBadGateway)

	FullForwarded(&ctx, &resp, "API", time.Millisecond)

	logs := output.String()

	assert.Contains(t, logs, `"level":"WARN"`)
	assert.Contains(t, logs, `"status":502`)
}

func TestFullForwardedIncludesTraceId(t *testing.T) {
	output := setTestLogger(t)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/test")
	ctx.SetUserValue("traceIdCtx", "test-trace-id")

	resp := fasthttp.Response{}
	resp.SetStatusCode(fasthttp.StatusOK)

	FullForwarded(&ctx, &resp, "API", time.Millisecond)

	logs := output.String()

	assert.Contains(t, logs, `"trace_id":"test-trace-id"`)
}

// TestFullForwardedUsesInboundPath asserts path comes from the client-facing ctx.Request,
// not a gateway-rewritten path.
func TestFullForwardedUsesInboundPath(t *testing.T) {
	output := setTestLogger(t)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetRequestURI("/api/wallets")

	resp := fasthttp.Response{}
	resp.SetStatusCode(fasthttp.StatusOK)

	FullForwarded(&ctx, &resp, "API", time.Millisecond)

	logs := output.String()

	assert.Contains(t, logs, `"path":"/api/wallets"`)
}
