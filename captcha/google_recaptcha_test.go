package captcha

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/mocks"
)

func TestVerify(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := mocks.NewHttpRetryClientMock(ctrl)

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "captcha_challenge_2")

	c.EXPECT().WithReadTimeout(gomock.Eq(googleApiReadTimeout))
	c.EXPECT().WithWriteTimeout(gomock.Eq(googleApiWriteTimeout))
	c.EXPECT().WithRetryAttempts(gomock.Eq(googleApiRetryAttempts))
	c.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(`{"success":true,"score":0.99,"error-codes":["no-error"]}`)

		assert.NotNil(t, req)
		assert.Equal(t, fasthttp.MethodPost, string(req.Header.Method()))
		assert.Equal(t, googleApiReCaptchaVerifyUrl, req.URI().String())
		assert.Equal(t, string(headers.ContentTypeForm), string(req.Header.ContentType()))
		assert.Equal(t, "captcha_secret_1", string(req.PostArgs().Peek("secret")))
		assert.Equal(t, "10.0.0.1", string(req.PostArgs().Peek("remoteip")))
		assert.Equal(t, "captcha_challenge_2", string(req.PostArgs().Peek("response")))

		return nil
	})

	p := NewGoogleReCaptchaProvider(c, config.Config{
		CaptchaSecret: "captcha_secret_1",
	})
	state, err := p.Verify(&ctx)

	assert.True(t, state)
	assert.NoError(t, err)
}

func TestVerifyUnsuccessful(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := mocks.NewHttpRetryClientMock(ctrl)

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "captcha_challenge_2")

	c.EXPECT().WithReadTimeout(gomock.Eq(googleApiReadTimeout))
	c.EXPECT().WithWriteTimeout(gomock.Eq(googleApiWriteTimeout))
	c.EXPECT().WithRetryAttempts(gomock.Eq(googleApiRetryAttempts))
	c.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(`{"success":false,"score":0.99,"error-codes":["bad-input"]}`)

		assert.NotNil(t, req)
		assert.Equal(t, fasthttp.MethodPost, string(req.Header.Method()))
		assert.Equal(t, googleApiReCaptchaVerifyUrl, req.URI().String())
		assert.Equal(t, string(headers.ContentTypeForm), string(req.Header.ContentType()))
		assert.Equal(t, "captcha_secret_1", string(req.PostArgs().Peek("secret")))
		assert.Equal(t, "10.0.0.1", string(req.PostArgs().Peek("remoteip")))
		assert.Equal(t, "captcha_challenge_2", string(req.PostArgs().Peek("response")))

		return nil
	})

	p := NewGoogleReCaptchaProvider(c, config.Config{
		CaptchaSecret: "captcha_secret_1",
	})
	state, err := p.Verify(&ctx)

	assert.False(t, state)
	assert.NoError(t, err)
}

func TestVerifyEmptySecret(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := mocks.NewHttpRetryClientMock(ctrl)

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "captcha_challenge_2")

	c.EXPECT().WithReadTimeout(gomock.Eq(googleApiReadTimeout))
	c.EXPECT().WithWriteTimeout(gomock.Eq(googleApiWriteTimeout))
	c.EXPECT().WithRetryAttempts(gomock.Eq(googleApiRetryAttempts))

	p := NewGoogleReCaptchaProvider(c, config.Config{
		CaptchaSecret: "",
	})
	state, err := p.Verify(&ctx)

	assert.True(t, state)
	assert.NoError(t, err)
}

func TestVerifyOptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := mocks.NewHttpRetryClientMock(ctrl)

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.SetMethod(fasthttp.MethodOptions)
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "captcha_challenge_2")

	c.EXPECT().WithReadTimeout(gomock.Eq(googleApiReadTimeout))
	c.EXPECT().WithWriteTimeout(gomock.Eq(googleApiWriteTimeout))
	c.EXPECT().WithRetryAttempts(gomock.Eq(googleApiRetryAttempts))

	p := NewGoogleReCaptchaProvider(c, config.Config{
		CaptchaSecret: "captcha_secret_1",
	})
	state, err := p.Verify(&ctx)

	assert.True(t, state)
	assert.NoError(t, err)
}

func TestVerifyEmptyChallenge(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := mocks.NewHttpRetryClientMock(ctrl)

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "")

	c.EXPECT().WithReadTimeout(gomock.Eq(googleApiReadTimeout))
	c.EXPECT().WithWriteTimeout(gomock.Eq(googleApiWriteTimeout))
	c.EXPECT().WithRetryAttempts(gomock.Eq(googleApiRetryAttempts))

	p := NewGoogleReCaptchaProvider(c, config.Config{
		CaptchaSecret: "captcha_secret_1",
	})
	state, err := p.Verify(&ctx)

	assert.False(t, state)
	assert.NoError(t, err)
}

func TestVerifyRequestFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := mocks.NewHttpRetryClientMock(ctrl)

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "captcha_challenge_2")

	c.EXPECT().WithReadTimeout(gomock.Eq(googleApiReadTimeout))
	c.EXPECT().WithWriteTimeout(gomock.Eq(googleApiWriteTimeout))
	c.EXPECT().WithRetryAttempts(gomock.Eq(googleApiRetryAttempts))
	c.EXPECT().Do(gomock.Any(), gomock.Any()).Return(fmt.Errorf("broken pipe"))

	p := NewGoogleReCaptchaProvider(c, config.Config{
		CaptchaSecret: "captcha_secret_1",
	})
	state, err := p.Verify(&ctx)

	assert.False(t, state)
	assert.Error(t, err)
}

func TestVerifyBadResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := mocks.NewHttpRetryClientMock(ctrl)

	ctx := fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "captcha_challenge_2")

	c.EXPECT().WithReadTimeout(gomock.Eq(googleApiReadTimeout))
	c.EXPECT().WithWriteTimeout(gomock.Eq(googleApiWriteTimeout))
	c.EXPECT().WithRetryAttempts(gomock.Eq(googleApiRetryAttempts))
	c.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(`{"success":true`)
		return nil
	})

	p := NewGoogleReCaptchaProvider(c, config.Config{
		CaptchaSecret: "captcha_secret_1",
	})
	state, err := p.Verify(&ctx)

	assert.False(t, state)
	assert.Error(t, err)
}
