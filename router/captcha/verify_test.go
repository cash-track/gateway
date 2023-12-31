package captcha

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
)

func TestVerify(t *testing.T) {
	newClient()

	config.Global.CaptchaSecret = "captcha_secret_1"

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "captcha_challenge_2")

	mock := &MockClient{}
	mock.MockResponse(func(resp *fasthttp.Response) {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(`{"success":true,"score":0.99,"error-codes":["no-error"]}`)
	})

	client = mock

	state, err := Verify(&ctx)

	assert.True(t, state)
	assert.NoError(t, err)

	assert.NotNil(t, mock.GetRequest())
	assert.Equal(t, fasthttp.MethodPost, string(mock.GetRequest().Header.Method()))
	assert.Equal(t, verifyUrl, mock.GetRequest().URI().String())
	assert.Equal(t, string(headers.ContentTypeForm), string(mock.GetRequest().Header.ContentType()))
	assert.Equal(t, "captcha_secret_1", string(mock.GetRequest().PostArgs().Peek("secret")))
	assert.Equal(t, "10.0.0.1", string(mock.GetRequest().PostArgs().Peek("remoteip")))
	assert.Equal(t, "captcha_challenge_2", string(mock.GetRequest().PostArgs().Peek("response")))
}

func TestVerifyUnsuccessful(t *testing.T) {
	config.Global.CaptchaSecret = "captcha_secret_1"

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "captcha_challenge_2")

	mock := &MockClient{}
	mock.MockResponse(func(resp *fasthttp.Response) {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(`{"success":false,"score":0.99,"error-codes":["bad-input"]}`)
	})

	client = mock

	state, err := Verify(&ctx)

	assert.False(t, state)
	assert.NoError(t, err)

	assert.NotNil(t, mock.GetRequest())
	assert.Equal(t, fasthttp.MethodPost, string(mock.GetRequest().Header.Method()))
	assert.Equal(t, verifyUrl, mock.GetRequest().URI().String())
	assert.Equal(t, string(headers.ContentTypeForm), string(mock.GetRequest().Header.ContentType()))
	assert.Equal(t, "captcha_secret_1", string(mock.GetRequest().PostArgs().Peek("secret")))
	assert.Equal(t, "10.0.0.1", string(mock.GetRequest().PostArgs().Peek("remoteip")))
	assert.Equal(t, "captcha_challenge_2", string(mock.GetRequest().PostArgs().Peek("response")))
}

func TestVerifyEmptySecret(t *testing.T) {
	config.Global.CaptchaSecret = ""

	ctx := fasthttp.RequestCtx{}

	state, err := Verify(&ctx)

	assert.True(t, state)
	assert.NoError(t, err)
}

func TestVerifyEmptyChallenge(t *testing.T) {
	config.Global.CaptchaSecret = "captcha_secret_1"

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "")

	state, err := Verify(&ctx)

	assert.False(t, state)
	assert.NoError(t, err)
}

func TestVerifyRequestFail(t *testing.T) {
	config.Global.CaptchaSecret = "captcha_secret_1"

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "captcha_challenge_2")

	mock := &MockClient{}
	mock.ReturnError(fmt.Errorf("broken pipe"))

	client = mock

	state, err := Verify(&ctx)

	assert.False(t, state)
	assert.Error(t, err)
}

func TestVerifyBadResponse(t *testing.T) {
	config.Global.CaptchaSecret = "captcha_secret_1"

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "captcha_challenge_2")

	mock := &MockClient{}
	mock.MockResponse(func(resp *fasthttp.Response) {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(`{"success":true`)
	})

	client = mock

	state, err := Verify(&ctx)

	assert.False(t, state)
	assert.Error(t, err)
}
