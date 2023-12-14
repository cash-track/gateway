package logger

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"testing"

	"github.com/cash-track/gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestDebugRequest(t *testing.T) {
	var output bytes.Buffer
	log.SetOutput(&output)

	config.Global.DebugHttp = false

	req := fasthttp.Request{}
	req.Header.Set("Host", "127.0.0.1")

	DebugRequest(&req, "API")

	logs := output.String()

	assert.NotContains(t, logs, "DEBUG REQ API")
	assert.NotContains(t, logs, "127.0.0.1")

	config.Global.DebugHttp = true

	DebugRequest(&req, "API")

	logs = output.String()

	assert.Contains(t, logs, "DEBUG REQ API")
	assert.Contains(t, logs, "127.0.0.1")

}

func TestDebugResponse(t *testing.T) {
	var output bytes.Buffer
	log.SetOutput(&output)

	config.Global.DebugHttp = false

	resp := fasthttp.Response{}
	resp.Header.Set("Host", "127.0.0.1")

	DebugResponse(&resp, "API")

	logs := output.String()

	assert.NotContains(t, logs, "DEBUG RESP API")
	assert.NotContains(t, logs, "127.0.0.1")

	config.Global.DebugHttp = true

	DebugResponse(&resp, "API")

	logs = output.String()

	assert.Contains(t, logs, "DEBUG RESP API")
	assert.Contains(t, logs, "127.0.0.1")

}

func TestDebugHandler(t *testing.T) {
	var output bytes.Buffer
	log.SetOutput(&output)
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.Set("Host", "127.0.0.1")
	ctx.Response.Header.Set("Host", "127.0.0.2")

	h := DebugHandler(func(ctx *fasthttp.RequestCtx) {})
	h(&ctx)

	logs := output.String()

	assert.Contains(t, logs, "DEBUG REQ")
	assert.Contains(t, logs, "127.0.0.1")
	assert.Contains(t, logs, "127.0.0.2")
}

func TestDebugHandlerIgnorePath(t *testing.T) {
	var output bytes.Buffer
	log.SetOutput(&output)
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.Request.URI().SetPath("/live")
	ctx.Request.Header.Set("Host", "127.0.0.1")
	ctx.Response.Header.Set("Host", "127.0.0.2")

	h := DebugHandler(func(ctx *fasthttp.RequestCtx) {})
	h(&ctx)

	logs := output.String()

	assert.NotContains(t, logs, "DEBUG REQ")
	assert.NotContains(t, logs, "127.0.0.1")
	assert.NotContains(t, logs, "127.0.0.2")
}

func TestFullForwarded(t *testing.T) {
	var output bytes.Buffer
	log.SetOutput(&output)
	config.Global.DebugHttp = true

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})

	req := fasthttp.Request{}
	req.Header.SetMethod(fasthttp.MethodPost)
	req.SetRequestURI("/test?one=two%203")
	req.SetBody([]byte("123"))

	resp := fasthttp.Response{}
	resp.SetStatusCode(fasthttp.StatusBadRequest)
	resp.SetBody([]byte("4567"))

	FullForwarded(&ctx, &req, &resp, "API")

	logs := output.String()

	fmt.Print(logs)

	assert.Contains(t, logs, "[10.0.0.1] POST /test?one=two%203 (body 3b) => API resp 400 (4b)")

}
