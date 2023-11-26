package headers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestWriteBearerToken(t *testing.T) {
	req := fasthttp.Request{}

	WriteBearerToken(&req, "secret")

	value := req.Header.Peek(Authorization)

	assert.Equal(t, "Bearer secret", string(value))
}
