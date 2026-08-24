package api

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/mocks"
	"github.com/cash-track/gateway/service/api/refresh"
)

// testCoordinator returns a tier-1-only coordinator (no Redis client) for tests that
// exercise ForwardRequest/Healthcheck/etc. without caring about refresh coalescing
// itself. See refresh/coordinator_test.go for coordinator-specific behaviour.
func testCoordinator() *refresh.RedisCoordinator {
	return refresh.NewRedis(nil, 0)
}

func TestRefreshLockTTLHasRealHeadroomOverWorstCaseCallDuration(t *testing.T) {
	// The lock must outlive refreshHttpTimeout (refreshToken's own worst case, not the
	// general forwarding timeouts) plus margin, or a second instance can acquire it
	// mid-refresh and duplicate the call.
	worstCase := refreshHttpTimeout

	got := RefreshLockTTL()

	assert.Greater(t, got, worstCase)
	assert.Equal(t, worstCase+refreshLockMargin, got)
}

// TestRefreshLockTTLStaysWellBelowSPARequestTimeout guards waitBudget against ever
// reaching the SPA's REQUEST_TIMEOUT_MS (15s, frontend/src/api/client.ts) — past that,
// the browser aborts before a legitimately slow leader responds. Fails loudly if the
// underlying timeout constants drift and erode the margin.
func TestRefreshLockTTLStaysWellBelowSPARequestTimeout(t *testing.T) {
	const spaRequestTimeout = 15 * time.Second

	lockTtl := RefreshLockTTL()
	waitBudget := refresh.NewRedis(nil, lockTtl).WaitBudget()

	assert.Less(t, waitBudget, spaRequestTimeout, "waitBudget must stay below the SPA's own client-side request timeout")

	const minMargin = 3 * time.Second
	assert.LessOrEqualf(t, waitBudget, spaRequestTimeout-minMargin,
		"waitBudget (%s) leaves less than the required %s margin under the SPA's %s request timeout",
		waitBudget, minMargin, spaRequestTimeout)
}

func TestNewClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))

	s := NewHttp(h, config.Config{}, nil, testBreaker(), testCoordinator())

	assert.NotNil(t, s.http)
}

func TestSetRequestURI(t *testing.T) {
	apiUrl, _ := url.Parse("http://api.test.com")

	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	}, nil, testBreaker(), testCoordinator())

	uri := fasthttp.URI{}

	s.setRequestURI(&uri, []byte("/users/create one"))

	assert.Equal(t, "http://api.test.com/users/create%20one", uri.String())
}

func TestCopyRequestURI(t *testing.T) {
	apiUrl, _ := url.Parse("http://api.test.com")

	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	}, nil, testBreaker(), testCoordinator())

	src := fasthttp.URI{}
	src.SetPath("/api/users/create one")
	src.SetQueryString("one=two%203")
	dest := fasthttp.URI{}

	s.copyRequestURI(&src, &dest)

	assert.Equal(t, "http://api.test.com/v1/users/create%20one?one=two%203", dest.String())
}
