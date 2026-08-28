package api

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/mocks"
)

func TestAuthActionFromPath(t *testing.T) {
	cases := map[string]string{
		"/api/auth/login":                authActionLogin,
		"/api/auth/login/passkey":        authActionPasskey,
		"/api/auth/register":             authActionRegister,
		"/api/auth/provider/google":      authActionGoogle,
		"/api/auth/something/unexpected": authActionLogin,
	}

	for path, want := range cases {
		assert.Equal(t, want, authActionFromPath([]byte(path)), path)
	}
}

func TestAuthResultFromStatus(t *testing.T) {
	assert.Equal(t, authResultSuccess, authResultFromStatus(fasthttp.StatusOK))
	assert.Equal(t, authResultSuccess, authResultFromStatus(fasthttp.StatusNoContent))
	assert.Equal(t, authResultFailure, authResultFromStatus(fasthttp.StatusFound))
	assert.Equal(t, authResultFailure, authResultFromStatus(fasthttp.StatusUnauthorized))
	assert.Equal(t, authResultFailure, authResultFromStatus(fasthttp.StatusBadGateway))
}

func TestIsPasskeyInitPath(t *testing.T) {
	assert.True(t, isPasskeyInitPath([]byte("/api/auth/login/passkey/init")))
	assert.False(t, isPasskeyInitPath([]byte("/api/auth/login/passkey")))
	assert.False(t, isPasskeyInitPath([]byte("/api/auth/login")))
}

// TestAuthSetHandlerRecordsAttemptMetric asserts a successful login advances the
// counter for action=login result=success.
func TestAuthSetHandlerRecordsAttemptMetric(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	c.EXPECT().Verify(gomock.Any()).Return(true, nil)
	s.EXPECT().ForwardRequest(gomock.Any(), nil).DoAndReturn(func(ctx *fasthttp.RequestCtx, _ []byte) error {
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.SetBodyString(`{}`)
		return nil
	})

	before := testutil.ToFloat64(authAttemptsTotal.WithLabelValues(authActionLogin, authResultSuccess))

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetRequestURI("/api/auth/login")

	h.AuthSetHandler(&ctx)

	assert.Equal(t, before+1, testutil.ToFloat64(authAttemptsTotal.WithLabelValues(authActionLogin, authResultSuccess)))
}

// TestAuthSetHandlerRecordsFailureOnCaptcha asserts a captcha rejection records a failure.
func TestAuthSetHandlerRecordsFailureOnCaptcha(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	c.EXPECT().Verify(gomock.Any()).Return(false, nil)

	before := testutil.ToFloat64(authAttemptsTotal.WithLabelValues(authActionPasskey, authResultFailure))

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetRequestURI("/api/auth/login/passkey")

	h.AuthSetHandler(&ctx)

	assert.Equal(t, before+1, testutil.ToFloat64(authAttemptsTotal.WithLabelValues(authActionPasskey, authResultFailure)))
}

// TestCaptchaVerifyHandlerRecordsPasskeyInit asserts the passkey/init route
// records under action=passkey_init and nothing double-counts for other paths.
func TestCaptchaVerifyHandlerRecordsPasskeyInit(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	c.EXPECT().Verify(gomock.Any()).Return(true, nil)
	s.EXPECT().ForwardRequest(gomock.Any(), nil).DoAndReturn(func(ctx *fasthttp.RequestCtx, _ []byte) error {
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		return nil
	})

	before := testutil.ToFloat64(authAttemptsTotal.WithLabelValues(authActionPasskeyInit, authResultSuccess))

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/api/auth/login/passkey/init")

	h.CaptchaVerifyHandler(&ctx)

	assert.Equal(t, before+1, testutil.ToFloat64(authAttemptsTotal.WithLabelValues(authActionPasskeyInit, authResultSuccess)))
}

// TestCaptchaVerifyHandlerNoMetricForNonInitPath asserts a bare CaptchaVerifyHandler
// call on a non-init path does not touch the passkey_init counter.
func TestCaptchaVerifyHandlerNoMetricForNonInitPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	c.EXPECT().Verify(gomock.Any()).Return(false, fmt.Errorf("boom"))

	before := testutil.ToFloat64(authAttemptsTotal.WithLabelValues(authActionPasskeyInit, authResultFailure))

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/api/auth/login/passkey")

	h.CaptchaVerifyHandler(&ctx)

	assert.Equal(t, before, testutil.ToFloat64(authAttemptsTotal.WithLabelValues(authActionPasskeyInit, authResultFailure)))
}
