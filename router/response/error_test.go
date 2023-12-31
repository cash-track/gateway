package response

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/headers"
)

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse("", nil, 0)

	assert.Empty(t, resp.Error)
}

func TestWriteByError(t *testing.T) {
	errorMsg := "broken pipe"

	resp := ByError(fmt.Errorf(errorMsg))

	ctx := fasthttp.RequestCtx{}

	resp.Write(&ctx)

	assert.Equal(t, fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
	assert.Equal(t, string(headers.ContentTypeJson), string(ctx.Response.Header.Peek(headers.ContentType)))
	assert.Equal(t, fmt.Sprintf(`{"message":"%s","error":"%s"}`, DefaultErrorMessage, errorMsg), string(ctx.Response.Body()))
}

func TestWriteByErrorAndStatus(t *testing.T) {
	errorMsg := "broken pipe"
	statusCode := fasthttp.StatusUnprocessableEntity

	resp := ByErrorAndStatus(fmt.Errorf(errorMsg), statusCode)

	ctx := fasthttp.RequestCtx{}

	resp.Write(&ctx)

	assert.Equal(t, statusCode, ctx.Response.StatusCode())
	assert.Equal(t, string(headers.ContentTypeJson), string(ctx.Response.Header.Peek(headers.ContentType)))
	assert.Equal(t, fmt.Sprintf(`{"message":"%s","error":"%s"}`, DefaultErrorMessage, errorMsg), string(ctx.Response.Body()))
}
