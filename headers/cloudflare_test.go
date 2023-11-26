package headers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestCopyCloudFlareHeaders(t *testing.T) {
	ctx := fasthttp.RequestCtx{}
	req := fasthttp.Request{}

	ctx.Request.Header.SetBytesV("Cf-Connection-Ip", []byte("192.168.1.1, 10.0.0.1"))
	ctx.Request.Header.SetBytesV("X-Request-Id", []byte("123456"))
	ctx.Request.Header.SetBytesV("CfIndex", []byte("qwerty"))

	CopyCloudFlareHeaders(&ctx, &req)

	headers := req.Header.String()

	fmt.Printf(headers)
	assert.Equal(t, "192.168.1.1, 10.0.0.1", string(req.Header.Peek("Cf-Original-Connection-Ip")))
	assert.Empty(t, req.Header.Peek("Cf-Connection-Ip"))
	assert.Empty(t, req.Header.Peek("X-Request-Id"))
	assert.Empty(t, req.Header.Peek("CfIndex"))
}
