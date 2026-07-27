package headers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestWriteGatewayVersionBothSet(t *testing.T) {
	req := fasthttp.Request{}

	WriteGatewayVersion(&req.Header, "v1.2.3", "abc123")

	assert.Equal(t, "v1.2.3", string(req.Header.Peek(XCtGatewayVersion)))
	assert.Equal(t, "abc123", string(req.Header.Peek(XCtGatewaySha)))
}

func TestWriteGatewayVersionBothEmpty(t *testing.T) {
	req := fasthttp.Request{}

	WriteGatewayVersion(&req.Header, "", "")

	assert.Empty(t, req.Header.Peek(XCtGatewayVersion))
	assert.Empty(t, req.Header.Peek(XCtGatewaySha))
}

func TestWriteGatewayVersionTagOnly(t *testing.T) {
	req := fasthttp.Request{}

	WriteGatewayVersion(&req.Header, "v1.2.3", "")

	assert.Equal(t, "v1.2.3", string(req.Header.Peek(XCtGatewayVersion)))
	assert.Empty(t, req.Header.Peek(XCtGatewaySha))
}

func TestWriteGatewayVersionShaOnly(t *testing.T) {
	req := fasthttp.Request{}

	WriteGatewayVersion(&req.Header, "", "abc123")

	assert.Empty(t, req.Header.Peek(XCtGatewayVersion))
	assert.Equal(t, "abc123", string(req.Header.Peek(XCtGatewaySha)))
}

func TestWriteGatewayVersionOnResponseHeader(t *testing.T) {
	resp := fasthttp.Response{}

	WriteGatewayVersion(&resp.Header, "v1.2.3", "abc123")

	assert.Equal(t, "v1.2.3", string(resp.Header.Peek(XCtGatewayVersion)))
	assert.Equal(t, "abc123", string(resp.Header.Peek(XCtGatewaySha)))
}
