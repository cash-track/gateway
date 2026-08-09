package api

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/mocks"
)

// testBreaker returns a breaker with production settings. NewBreaker registers no
// metrics, so each test can have its own.
func testBreaker() *gobreaker.CircuitBreaker[struct{}] {
	return NewBreaker()
}

func newTestService(t *testing.T, breaker *gobreaker.CircuitBreaker[struct{}]) (*HttpService, *mocks.HttpRetryClientMock) {
	t.Helper()

	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{ApiURI: apiUrl}, nil, breaker)

	return s, h
}

func TestDoWithBreakerOpensAfterConsecutiveTransportErrors(t *testing.T) {
	transportErr := errors.New("connection refused")
	s, h := newTestService(t, testBreaker())

	// breakerFailureThreshold consecutive failures keep the breaker closed; the next
	// one trips it.
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(transportErr).Times(breakerFailureThreshold + 1)

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	var err error
	for i := 0; i < breakerFailureThreshold+1; i++ {
		err = s.doWithBreaker(req, resp)
		assert.ErrorIs(t, err, transportErr)
	}

	assert.Equal(t, gobreaker.StateOpen, s.breaker.State())

	// Now open: rejected without reaching the transport, which the mock's call count proves.
	err = s.doWithBreaker(req, resp)
	assert.ErrorIs(t, err, ErrCircuitOpen)
}

func TestDoWithBreakerStaysClosedOn5xx(t *testing.T) {
	s, h := newTestService(t, testBreaker())

	attempts := breakerFailureThreshold + 5
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusInternalServerError)

		return nil
	}).Times(attempts)

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	for i := 0; i < attempts; i++ {
		err := s.doWithBreaker(req, resp)
		assert.NoError(t, err)
		assert.Equal(t, fasthttp.StatusInternalServerError, resp.StatusCode())
	}

	assert.Equal(t, gobreaker.StateClosed, s.breaker.State())
}

func TestForwardRequestErrorIsErrCircuitOpenWhenBreakerOpen(t *testing.T) {
	transportErr := errors.New("connection refused")
	breaker := testBreaker()
	s, h := newTestService(t, breaker)

	h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(transportErr).Times(breakerFailureThreshold + 1)

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	for i := 0; i < breakerFailureThreshold+1; i++ {
		_ = s.doWithBreaker(req, resp)
	}

	assert.Equal(t, gobreaker.StateOpen, breaker.State())

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)

	err := s.ForwardRequest(&ctx, nil)

	assert.ErrorIs(t, err, ErrCircuitOpen)
}

func TestHealthcheckBypassesBreaker(t *testing.T) {
	// A breaker that opens on the very first failure, so an open state is trivial to reach.
	breaker := gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 1
		},
	})

	s, h := newTestService(t, breaker)

	tripReq := fasthttp.AcquireRequest()
	tripResp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(tripReq)
	defer fasthttp.ReleaseResponse(tripResp)

	gomock.InOrder(
		h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(errors.New("connection refused")),
		h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
			resp.SetStatusCode(fasthttp.StatusOK)

			return nil
		}),
	)

	err := s.doWithBreaker(tripReq, tripResp)
	assert.Error(t, err)
	assert.Equal(t, gobreaker.StateOpen, breaker.State())

	// Healthcheck talks to the API directly and must succeed despite the open breaker.
	err = s.Healthcheck()
	assert.NoError(t, err)
}

func TestRegisterBreakerMetricsExposesState(t *testing.T) {
	breaker := testBreaker()
	RegisterBreakerMetrics(breaker)

	expected := `
# HELP gateway_api_breaker_state API circuit breaker state: 0=closed, 1=half-open, 2=open.
# TYPE gateway_api_breaker_state gauge
gateway_api_breaker_state 0
`
	assert.NoError(t, testutil.GatherAndCompare(prometheus.DefaultGatherer, strings.NewReader(expected), "gateway_api_breaker_state"))
}

func TestBreakerHalfOpenRecovery(t *testing.T) {
	breaker := gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		MaxRequests: 1,
		Timeout:     20 * time.Millisecond,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 1
		},
	})

	s, h := newTestService(t, breaker)

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	gomock.InOrder(
		h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(errors.New("connection refused")),
		h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
			resp.SetStatusCode(fasthttp.StatusOK)

			return nil
		}),
	)

	// 1st call fails and trips the breaker open.
	err := s.doWithBreaker(req, resp)
	assert.Error(t, err)
	assert.Equal(t, gobreaker.StateOpen, breaker.State())

	// Still within Timeout: rejected without reaching the transport.
	err = s.doWithBreaker(req, resp)
	assert.ErrorIs(t, err, ErrCircuitOpen)

	time.Sleep(40 * time.Millisecond)

	// Past Timeout: breaker probes with a single request, which succeeds.
	err = s.doWithBreaker(req, resp)
	assert.NoError(t, err)
	assert.Equal(t, gobreaker.StateClosed, breaker.State())
}
