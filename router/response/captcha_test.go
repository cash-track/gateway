package response

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestNewCaptchaBadResponse(t *testing.T) {
	resp := NewCaptchaBadResponse()

	assert.Equal(t, "Captcha validation unsuccessful. Please try again.", resp.Message)
	assert.Empty(t, resp.Error)
	assert.Equal(t, fasthttp.StatusBadRequest, resp.StatusCode)
}

func TestNewCaptchaErrorResponse(t *testing.T) {
	resp := NewCaptchaErrorResponse(fmt.Errorf("broken pipe"))

	assert.Equal(t, "Unexpected response from captcha validation service. Please try again later.", resp.Message)
	assert.Equal(t, "broken pipe", resp.Error)
	assert.Equal(t, fasthttp.StatusInternalServerError, resp.StatusCode)
}
