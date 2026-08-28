package captcha

import (
	"fmt"
	"net"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/mocks"
)

func newMetricsTestCtx() *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "challenge")

	return ctx
}

func newMetricsTestProvider(t *testing.T, secret string) (*GoogleReCaptchaProvider, *mocks.HttpRetryClientMock) {
	t.Helper()

	ctrl := gomock.NewController(t)
	c := mocks.NewHttpRetryClientMock(ctrl)
	c.EXPECT().WithReadTimeout(gomock.Any())
	c.EXPECT().WithWriteTimeout(gomock.Any())
	c.EXPECT().WithRetryAttempts(gomock.Any())

	return NewGoogleReCaptchaProvider(c, config.Config{CaptchaSecret: secret}), c
}

func TestVerifyMetricSolvedRecordsResultAndScore(t *testing.T) {
	p, c := newMetricsTestProvider(t, "secret")
	c.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(`{"success":true,"score":0.9}`)
		return nil
	})

	resultBefore := testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultSolved))
	scoreBefore := testutil.CollectAndCount(captchaScore)

	ok, err := p.Verify(newMetricsTestCtx())

	assert.True(t, ok)
	assert.NoError(t, err)
	assert.Equal(t, resultBefore+1, testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultSolved)))
	assert.GreaterOrEqual(t, testutil.CollectAndCount(captchaScore), scoreBefore)
}

func TestVerifyMetricMissedOnUnsuccessful(t *testing.T) {
	p, c := newMetricsTestProvider(t, "secret")
	c.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(`{"success":false}`)
		return nil
	})

	before := testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultMissed))

	ok, err := p.Verify(newMetricsTestCtx())

	assert.False(t, ok)
	assert.NoError(t, err)
	assert.Equal(t, before+1, testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultMissed)))
}

func TestVerifyMetricMissedOnEmptyChallenge(t *testing.T) {
	p, _ := newMetricsTestProvider(t, "secret")

	ctx := newMetricsTestCtx()
	ctx.Request.Header.Set(headers.XCtCaptchaChallenge, "")

	before := testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultMissed))

	ok, err := p.Verify(ctx)

	assert.False(t, ok)
	assert.NoError(t, err)
	assert.Equal(t, before+1, testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultMissed)))
}

func TestVerifyMetricErrorOnTransportFailure(t *testing.T) {
	p, c := newMetricsTestProvider(t, "secret")
	c.EXPECT().Do(gomock.Any(), gomock.Any()).Return(fmt.Errorf("broken pipe"))

	before := testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultError))

	_, err := p.Verify(newMetricsTestCtx())

	assert.Error(t, err)
	assert.Equal(t, before+1, testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultError)))
}

func TestVerifyMetricErrorOnBadBody(t *testing.T) {
	p, c := newMetricsTestProvider(t, "secret")
	c.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(`{"success":true`)
		return nil
	})

	before := testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultError))

	_, err := p.Verify(newMetricsTestCtx())

	assert.Error(t, err)
	assert.Equal(t, before+1, testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultError)))
}

func TestVerifyMetricDisabledOnEmptySecret(t *testing.T) {
	p, _ := newMetricsTestProvider(t, "")

	before := testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultDisabled))

	ok, err := p.Verify(newMetricsTestCtx())

	assert.True(t, ok)
	assert.NoError(t, err)
	assert.Equal(t, before+1, testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultDisabled)))
}

func TestVerifyMetricDisabledOnOptions(t *testing.T) {
	p, _ := newMetricsTestProvider(t, "secret")

	ctx := newMetricsTestCtx()
	ctx.Request.Header.SetMethod(fasthttp.MethodOptions)

	before := testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultDisabled))

	ok, err := p.Verify(ctx)

	assert.True(t, ok)
	assert.NoError(t, err)
	assert.Equal(t, before+1, testutil.ToFloat64(captchaVerificationsTotal.WithLabelValues(resultDisabled)))
}

func TestObserveCaptchaScoreIgnoresZero(t *testing.T) {
	before := testutil.CollectAndCount(captchaScore)
	observeCaptchaScore(0)
	assert.Equal(t, before, testutil.CollectAndCount(captchaScore))
}
