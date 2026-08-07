package headers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestWriteGatewaySecretSet(t *testing.T) {
	req := fasthttp.Request{}

	WriteGatewaySecret(&req.Header, "shared-secret")

	assert.Equal(t, "shared-secret", string(req.Header.Peek(XGatewaySecret)))
}

func TestWriteGatewaySecretEmpty(t *testing.T) {
	req := fasthttp.Request{}

	WriteGatewaySecret(&req.Header, "")

	assert.Empty(t, req.Header.Peek(XGatewaySecret))
}
