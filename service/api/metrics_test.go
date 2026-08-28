package api

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/mocks"
)

func TestWithInFlightBalancesGaugeAndPropagatesResult(t *testing.T) {
	before := testutil.ToFloat64(upstreamRequestsInFlight)

	wantErr := fmt.Errorf("boom")
	gotErr := withInFlight(upstreamRequestsInFlight, func() error {
		// The gauge is incremented for the duration of fn.
		assert.Equal(t, before+1, testutil.ToFloat64(upstreamRequestsInFlight))
		return wantErr
	})

	assert.Equal(t, wantErr, gotErr)
	assert.Equal(t, before, testutil.ToFloat64(upstreamRequestsInFlight))
}

func TestWithInFlightDecrementsOnPanic(t *testing.T) {
	before := testutil.ToFloat64(upstreamRequestsInFlight)

	assert.PanicsWithValue(t, "kaboom", func() {
		_ = withInFlight(upstreamRequestsInFlight, func() error {
			panic("kaboom")
		})
	})

	assert.Equal(t, before, testutil.ToFloat64(upstreamRequestsInFlight))
}

func TestStatusClass(t *testing.T) {
	cases := map[int]string{
		200: "2xx",
		204: "2xx",
		301: "3xx",
		404: "4xx",
		503: "5xx",
		100: "other",
		600: "other",
	}

	for code, want := range cases {
		assert.Equal(t, want, statusClass(code), "status %d", code)
	}
}

func TestUpstreamMethod(t *testing.T) {
	for _, m := range []string{
		fasthttp.MethodGet, fasthttp.MethodHead, fasthttp.MethodPost, fasthttp.MethodPut,
		fasthttp.MethodPatch, fasthttp.MethodDelete, fasthttp.MethodOptions,
	} {
		assert.Equal(t, m, upstreamMethod(m))
	}

	assert.Equal(t, "other", upstreamMethod("PROPFIND"))
	assert.Equal(t, "other", upstreamMethod(""))
}

func TestObserveUpstreamRecordsDurationAndStatusClass(t *testing.T) {
	before := testutil.ToFloat64(upstreamRequestsTotal.WithLabelValues(fasthttp.MethodGet, "2xx"))
	durBefore := testutil.CollectAndCount(upstreamRequestDuration)

	observeUpstream(fasthttp.MethodGet, 200, 0.05)

	assert.Equal(t, before+1, testutil.ToFloat64(upstreamRequestsTotal.WithLabelValues(fasthttp.MethodGet, "2xx")))
	assert.GreaterOrEqual(t, testutil.CollectAndCount(upstreamRequestDuration), durBefore)
}

// TestForwardRequestRecordsUpstreamSuccessMetrics drives one plain forward and
// asserts the upstream counter advanced for the observed status class.
func TestForwardRequestRecordsUpstreamSuccessMetrics(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Any())
	h.EXPECT().WithWriteTimeout(gomock.Any())
	h.EXPECT().WithRetryAttempts(gomock.Any())
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		return nil
	})

	apiURL, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{ApiURI: apiURL}, nil, testBreaker(), testCoordinator())

	before := testutil.ToFloat64(upstreamRequestsTotal.WithLabelValues(fasthttp.MethodGet, "2xx"))

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)

	assert.NoError(t, s.ForwardRequest(&ctx, nil))
	assert.Equal(t, before+1, testutil.ToFloat64(upstreamRequestsTotal.WithLabelValues(fasthttp.MethodGet, "2xx")))
}

// TestForwardRequestRecordsUpstreamErrorMetric asserts the "error" status label
// is used when the forwarded call fails before any response.
func TestForwardRequestRecordsUpstreamErrorMetric(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Any())
	h.EXPECT().WithWriteTimeout(gomock.Any())
	h.EXPECT().WithRetryAttempts(gomock.Any())
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(fmt.Errorf("connection refused"))

	apiURL, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{ApiURI: apiURL}, nil, testBreaker(), testCoordinator())

	before := testutil.ToFloat64(upstreamRequestsTotal.WithLabelValues(fasthttp.MethodPost, upstreamStatusError))

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)

	assert.Error(t, s.ForwardRequest(&ctx, nil))
	assert.Equal(t, before+1, testutil.ToFloat64(upstreamRequestsTotal.WithLabelValues(fasthttp.MethodPost, upstreamStatusError)))
}

// TestForwardRequestRecordsTokenRefreshSuccess covers the success + retry path.
func TestForwardRequestRecordsTokenRefreshSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Any())
	h.EXPECT().WithWriteTimeout(gomock.Any())
	h.EXPECT().WithRetryAttempts(gomock.Any())
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusUnauthorized)
		return nil
	})
	h.EXPECT().DoTimeout(gomock.Any(), gomock.Any(), gomock.Eq(refreshHttpTimeout)).DoAndReturn(
		func(_ *fasthttp.Request, resp *fasthttp.Response, _ time.Duration) error {
			resp.SetStatusCode(fasthttp.StatusOK)
			resp.SetBodyString(fmt.Sprintf(`{"accessToken":"a","refreshToken":"r","refreshTokenExpiredAt":"%s"}`, tomorrowRFC3339()))
			return nil
		})
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		return nil
	})

	apiURL, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{ApiURI: apiURL}, nil, testBreaker(), testCoordinator())

	before := testutil.ToFloat64(tokenRefreshTotal.WithLabelValues(tokenRefreshSuccess))
	durBefore := testutil.CollectAndCount(tokenRefreshDuration)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, "access_token")
	ctx.Request.Header.SetCookie(cookie.RefreshTokenCookieName, "refresh_token")

	assert.NoError(t, s.ForwardRequest(&ctx, nil))
	assert.Equal(t, before+1, testutil.ToFloat64(tokenRefreshTotal.WithLabelValues(tokenRefreshSuccess)))
	assert.GreaterOrEqual(t, testutil.CollectAndCount(tokenRefreshDuration), durBefore)
}

// TestForwardRequestRecordsTokenRefreshFailed covers a genuinely expired refresh token.
func TestForwardRequestRecordsTokenRefreshFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Any())
	h.EXPECT().WithWriteTimeout(gomock.Any())
	h.EXPECT().WithRetryAttempts(gomock.Any())
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusUnauthorized)
		return nil
	})
	h.EXPECT().DoTimeout(gomock.Any(), gomock.Any(), gomock.Eq(refreshHttpTimeout)).DoAndReturn(
		func(_ *fasthttp.Request, resp *fasthttp.Response, _ time.Duration) error {
			resp.SetStatusCode(fasthttp.StatusUnauthorized)
			return nil
		})

	apiURL, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{ApiURI: apiURL}, nil, testBreaker(), testCoordinator())

	before := testutil.ToFloat64(tokenRefreshTotal.WithLabelValues(tokenRefreshFailed))

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, "access_token")
	ctx.Request.Header.SetCookie(cookie.RefreshTokenCookieName, "refresh_token")

	assert.NoError(t, s.ForwardRequest(&ctx, nil))
	assert.Equal(t, before+1, testutil.ToFloat64(tokenRefreshTotal.WithLabelValues(tokenRefreshFailed)))
}

// TestForwardRequestRecordsTokenRefreshSessionPreserved covers a transient API
// failure during refresh: cookies kept, 503 returned.
func TestForwardRequestRecordsTokenRefreshSessionPreserved(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Any())
	h.EXPECT().WithWriteTimeout(gomock.Any())
	h.EXPECT().WithRetryAttempts(gomock.Any())
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusUnauthorized)
		return nil
	})
	h.EXPECT().DoTimeout(gomock.Any(), gomock.Any(), gomock.Eq(refreshHttpTimeout)).Return(fmt.Errorf("api down"))

	apiURL, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{ApiURI: apiURL}, nil, testBreaker(), testCoordinator())

	before := testutil.ToFloat64(tokenRefreshTotal.WithLabelValues(tokenRefreshSessionPreserved))

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, "access_token")
	ctx.Request.Header.SetCookie(cookie.RefreshTokenCookieName, "refresh_token")

	assert.NoError(t, s.ForwardRequest(&ctx, nil))
	assert.Equal(t, fasthttp.StatusServiceUnavailable, ctx.Response.StatusCode())
	assert.Equal(t, before+1, testutil.ToFloat64(tokenRefreshTotal.WithLabelValues(tokenRefreshSessionPreserved)))
}
