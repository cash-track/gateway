package jwks

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/mocks"
)

const testEndpoint = "http://api.test.com"

func testConfig() config.Config {
	apiUrl, _ := url.Parse(testEndpoint)

	return config.Config{
		ApiUrl: testEndpoint,
		ApiURI: apiUrl,
		GitTag: "v1.2.3",
		GitSha: "abc123",
	}
}

func newTestProvider(t *testing.T) (*HttpProvider, *mocks.HttpRetryClientMock) {
	t.Helper()

	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))

	return NewHttp(h, testConfig()), h
}

// jwkFromKey builds a valid RFC 7517 JWK entry (RSA, base64url n/e) for a public key.
func jwkFromKey(kid string, pub *rsa.PublicKey) jwkKey {
	eBytes := big.NewInt(int64(pub.E)).Bytes()

	return jwkKey{
		Kty: "RSA",
		Kid: kid,
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(eBytes),
	}
}

func generateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	return key
}

func jsonBody(t *testing.T, v any) []byte {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal test body: %v", err)
	}

	return b
}

func TestNewHttpStartsWithEmptyUnloadedKeySet(t *testing.T) {
	p, _ := newTestProvider(t)

	assert.False(t, p.Loaded())

	key, ok := p.lookup("anything")
	assert.False(t, ok)
	assert.Nil(t, key)
}

func TestFetchSuccessParsesAndStoresKeys(t *testing.T) {
	p, h := newTestProvider(t)
	key := generateKey(t)

	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		assert.Equal(t, fasthttp.MethodGet, string(req.Header.Method()))
		assert.Equal(t, fmt.Sprintf("%s%s", testEndpoint, jwksURI), req.URI().String())
		assert.Equal(t, string(headers.ContentTypeJson), string(req.Header.ContentType()))
		assert.Equal(t, string(headers.ContentTypeJson), string(req.Header.Peek(headers.Accept)))
		assert.Equal(t, "v1.2.3", string(req.Header.Peek(headers.XCtGatewayVersion)))
		assert.Equal(t, "abc123", string(req.Header.Peek(headers.XCtGatewaySha)))

		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{jwkFromKey("kid-1", &key.PublicKey)}}))

		return nil
	})

	err := p.fetch()

	assert.NoError(t, err)
	assert.True(t, p.Loaded())

	got, ok := p.Key("kid-1")
	assert.True(t, ok)
	assert.Equal(t, key.PublicKey.N, got.N)
	assert.Equal(t, key.PublicKey.E, got.E)
}

func TestFetchHttpError(t *testing.T) {
	p, h := newTestProvider(t)
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(assert.AnError)

	err := p.fetch()

	assert.Error(t, err)
	assert.False(t, p.Loaded())
}

func TestFetchNon200(t *testing.T) {
	p, h := newTestProvider(t)
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusInternalServerError)

		return nil
	})

	err := p.fetch()

	assert.Error(t, err)
	assert.False(t, p.Loaded())
}

func TestFetchMalformedJSON(t *testing.T) {
	p, h := newTestProvider(t)
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody([]byte("{not json"))

		return nil
	})

	err := p.fetch()

	assert.Error(t, err)
	assert.False(t, p.Loaded())
}

func TestFetchEmptyKeySet(t *testing.T) {
	p, h := newTestProvider(t)
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{}}))

		return nil
	})

	err := p.fetch()

	assert.NoError(t, err)
	assert.False(t, p.Loaded())
}

func TestFetchSkipsNonRSAAndMalformedEntries(t *testing.T) {
	p, h := newTestProvider(t)
	key := generateKey(t)

	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{
			{Kty: "EC", Kid: "ec-kid", N: "irrelevant", E: "irrelevant"},
			{Kty: "RSA", Kid: "bad-n", N: "not-base64url!!", E: "AQAB"},
			{Kty: "RSA", Kid: "bad-e", N: base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()), E: "not-base64url!!"},
			{Kty: "RSA", Kid: "empty-n", N: "", E: "AQAB"},
			jwkFromKey("good-kid", &key.PublicKey),
		}}))

		return nil
	})

	err := p.fetch()

	assert.NoError(t, err)
	assert.True(t, p.Loaded())

	_, ok := p.lookup("ec-kid")
	assert.False(t, ok)
	_, ok = p.lookup("bad-n")
	assert.False(t, ok)
	_, ok = p.lookup("bad-e")
	assert.False(t, ok)
	_, ok = p.lookup("empty-n")
	assert.False(t, ok)
	_, ok = p.lookup("good-kid")
	assert.True(t, ok)
}

func TestFetchFailureLeavesPreviousKeysUntouched(t *testing.T) {
	p, h := newTestProvider(t)
	key := generateKey(t)

	first := h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{jwkFromKey("kid-1", &key.PublicKey)}}))

		return nil
	})
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(assert.AnError).After(first)

	assert.NoError(t, p.fetch())
	assert.True(t, p.Loaded())

	assert.Error(t, p.fetch())

	_, ok := p.lookup("kid-1")
	assert.True(t, ok)
}

// TestFetchEmptyResponseAfterLoadedKeepsExistingKeys is the regression test for the
// blocker found in round-1 review: a well-formed-but-empty 200 (a transient glitch, a
// stale cache, a race during rotation, a bad API deploy) must NOT permanently flip
// Loaded() from true back to false. Doing so would silently and irreversibly disable
// RS256 verification for the rest of the process - nothing in router/csrf can ever call
// Key()/fetch() again once Loaded() goes false, because keyfunc is only reachable from
// inside the "Loaded() is true" branch of parseClaims.
func TestFetchEmptyResponseAfterLoadedKeepsExistingKeys(t *testing.T) {
	p, h := newTestProvider(t)
	key := generateKey(t)

	loaded := h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{jwkFromKey("kid-1", &key.PublicKey)}}))

		return nil
	})
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{}}))

		return nil
	}).After(loaded)

	assert.NoError(t, p.fetch())
	assert.True(t, p.Loaded())

	// The empty response is not an error - fetch succeeds - but must be a no-op on the
	// key store rather than a regression to "no keys".
	assert.NoError(t, p.fetch())
	assert.True(t, p.Loaded())

	got, ok := p.lookup("kid-1")
	assert.True(t, ok)
	assert.Equal(t, key.PublicKey.N, got.N)
}

// TestFetchAllInvalidEntriesAfterLoadedKeepsExistingKeys covers the other way to reach an
// empty parsed set: a non-empty "keys" array where every entry is skipped (non-RSA or
// malformed n/e). Same blocker, same fix, different trigger.
func TestFetchAllInvalidEntriesAfterLoadedKeepsExistingKeys(t *testing.T) {
	p, h := newTestProvider(t)
	key := generateKey(t)

	loaded := h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{jwkFromKey("kid-1", &key.PublicKey)}}))

		return nil
	})
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{
			{Kty: "EC", Kid: "ec-kid", N: "irrelevant", E: "irrelevant"},
			{Kty: "RSA", Kid: "bad-n", N: "not-base64url!!", E: "AQAB"},
		}}))

		return nil
	}).After(loaded)

	assert.NoError(t, p.fetch())
	assert.True(t, p.Loaded())

	assert.NoError(t, p.fetch())
	assert.True(t, p.Loaded())

	got, ok := p.lookup("kid-1")
	assert.True(t, ok)
	assert.Equal(t, key.PublicKey.N, got.N)

	_, ok = p.lookup("ec-kid")
	assert.False(t, ok)
}

func TestParseRSAPublicKey(t *testing.T) {
	key := generateKey(t)
	validN := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
	validE := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())

	for name, test := range map[string]struct {
		n, e    string
		wantErr bool
	}{
		"Valid":              {n: validN, e: validE},
		"InvalidBase64N":     {n: "not-base64!!", e: validE, wantErr: true},
		"InvalidBase64E":     {n: validN, e: "not-base64!!", wantErr: true},
		"EmptyN":             {n: "", e: validE, wantErr: true},
		"EmptyE":             {n: validN, e: "", wantErr: true},
		"ZeroExponent":       {n: validN, e: base64.RawURLEncoding.EncodeToString([]byte{0}), wantErr: true},
		"ExponentOutOfRange": {n: validN, e: base64.RawURLEncoding.EncodeToString(bigExponentBytes()), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			pub, err := parseRSAPublicKey(test.n, test.e)

			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, pub)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, key.PublicKey.N, pub.N)
			assert.Equal(t, key.PublicKey.E, pub.E)
		})
	}
}

// bigExponentBytes returns a big-endian byte slice representing a value too large to fit
// in an int64, used to exercise parseRSAPublicKey's exponent-out-of-range branch.
func bigExponentBytes() []byte {
	b := make([]byte, 16)
	for i := range b {
		b[i] = 0xFF
	}

	return b
}

func TestKeyReturnsLoadedKey(t *testing.T) {
	p, h := newTestProvider(t)
	key := generateKey(t)

	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{jwkFromKey("kid-1", &key.PublicKey)}}))

		return nil
	})

	assert.NoError(t, p.fetch())

	got, ok := p.Key("kid-1")
	assert.True(t, ok)
	assert.Equal(t, key.PublicKey.N, got.N)
}

func TestKeyUnknownKidTriggersRefreshAndFinds(t *testing.T) {
	p, h := newTestProvider(t)
	key := generateKey(t)

	// The on-demand refresh triggered by the unknown kid is the only Do call: no prior
	// fetch ever succeeded, so lastRefreshAttempt is still zero (never rate-limited).
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{jwkFromKey("rotated-kid", &key.PublicKey)}}))

		return nil
	})

	got, ok := p.Key("rotated-kid")

	assert.True(t, ok)
	assert.Equal(t, key.PublicKey.N, got.N)
}

func TestKeyUnknownKidStillNotFoundAfterRefresh(t *testing.T) {
	p, h := newTestProvider(t)
	key := generateKey(t)

	h.EXPECT().Do(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{jwkFromKey("some-other-kid", &key.PublicKey)}}))

		return nil
	})

	got, ok := p.Key("missing-kid")

	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestKeyUnknownKidRefreshRequestErrorStillReportsNotFound(t *testing.T) {
	p, h := newTestProvider(t)
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Times(1).Return(assert.AnError)

	got, ok := p.Key("missing-kid")

	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestKeyRefreshIsRateLimited(t *testing.T) {
	p, h := newTestProvider(t)
	key := generateKey(t)

	// Exactly one Do call expected: the second Key() call for the same still-unknown kid
	// must be rate-limited and must NOT trigger a second fetch.
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{jwkFromKey("other-kid", &key.PublicKey)}}))

		return nil
	})

	_, ok := p.Key("missing-kid")
	assert.False(t, ok)

	_, ok = p.Key("missing-kid")
	assert.False(t, ok)
}

// withShrunkBootstrapBackoff temporarily lowers the bootstrap retry backoff so tests can
// wait out several real retry cycles - including the cap - in milliseconds instead of
// tens of seconds. Restores the originals on cleanup.
func withShrunkBootstrapBackoff(t *testing.T, min, max time.Duration) {
	t.Helper()

	origMin, origMax := bootstrapBackoffMin, bootstrapBackoffMax
	bootstrapBackoffMin, bootstrapBackoffMax = min, max

	t.Cleanup(func() {
		bootstrapBackoffMin, bootstrapBackoffMax = origMin, origMax
	})
}

func TestBootstrapRetriesThroughAllBranchesThenLoads(t *testing.T) {
	withShrunkBootstrapBackoff(t, 5*time.Millisecond, 20*time.Millisecond)

	p, h := newTestProvider(t)
	key := generateKey(t)

	callErr := h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(assert.AnError)
	callEmpty := h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{}}))

		return nil
	}).After(callErr)
	// Two more failures at the (already capped) max backoff, to actually exercise the
	// "backoff > bootstrapBackoffMax" clamp branch before finally succeeding.
	callCap1 := h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(assert.AnError).After(callEmpty)
	callCap2 := h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(assert.AnError).After(callCap1)
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{jwkFromKey("kid-1", &key.PublicKey)}}))

		return nil
	}).After(callCap2)

	// Genuinely waits through multiple retry cycles (backoff shrunk above) to exercise
	// the failure branch, the succeeded-but-empty branch, the backoff-exceeds-cap clamp,
	// and the terminal succeeded-and-loaded return - all real behaviour of the loop.
	ok := p.bootstrap(context.Background())

	assert.True(t, ok)
	assert.True(t, p.Loaded())
}

func TestBootstrapStopsOnContextCancellation(t *testing.T) {
	p, h := newTestProvider(t)
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(assert.AnError)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// ctx is already cancelled, so bootstrap attempts exactly one fetch, then returns via
	// the ctx.Done() branch instead of waiting out the backoff timer.
	ok := p.bootstrap(ctx)

	assert.False(t, ok)
	assert.False(t, p.Loaded())
}

// withShrunkPeriodicRefreshInterval mirrors withShrunkBootstrapBackoff, for the
// steady-state periodic refresh loop.
func withShrunkPeriodicRefreshInterval(t *testing.T, interval time.Duration) {
	t.Helper()

	orig := periodicRefreshInterval
	periodicRefreshInterval = interval

	t.Cleanup(func() {
		periodicRefreshInterval = orig
	})
}

func TestPeriodicRefreshTicksAndStopsOnContextCancellation(t *testing.T) {
	withShrunkPeriodicRefreshInterval(t, 5*time.Millisecond)

	p, h := newTestProvider(t)
	key := generateKey(t)

	h.EXPECT().Do(gomock.Any(), gomock.Any()).MinTimes(1).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{jwkFromKey("kid-1", &key.PublicKey)}}))

		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.periodicRefresh(ctx)
		close(done)
	}()

	// Let at least one tick fire before stopping.
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("periodicRefresh did not stop after context cancellation")
	}

	assert.True(t, p.Loaded())
}

func TestPeriodicRefreshLogsAndContinuesOnFetchError(t *testing.T) {
	withShrunkPeriodicRefreshInterval(t, 5*time.Millisecond)

	p, h := newTestProvider(t)
	h.EXPECT().Do(gomock.Any(), gomock.Any()).MinTimes(1).Return(assert.AnError)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.periodicRefresh(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("periodicRefresh did not stop after context cancellation")
	}

	assert.False(t, p.Loaded())
}

func TestRunReturnsWithoutPeriodicRefreshWhenBootstrapNeverSucceeds(t *testing.T) {
	p, h := newTestProvider(t)
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(assert.AnError)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// ctx is already cancelled: bootstrap fails fast and run must return immediately
	// rather than falling through into periodicRefresh (which would need a live ctx and
	// would otherwise tick forever / call Do again unexpectedly against this mock).
	p.run(ctx)

	assert.False(t, p.Loaded())
}

func TestStartLoadsKeysInBackground(t *testing.T) {
	p, h := newTestProvider(t)
	key := generateKey(t)

	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{jwkFromKey("kid-1", &key.PublicKey)}}))

		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for !p.Loaded() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	assert.True(t, p.Loaded())
}

func TestConcurrentReadsAndRefresh(t *testing.T) {
	p, h := newTestProvider(t)
	key := generateKey(t)

	h.EXPECT().Do(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBody(jsonBody(t, jwksResponse{Keys: []jwkKey{jwkFromKey("kid-1", &key.PublicKey)}}))

		return nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < 50; j++ {
				p.Key("kid-1")
				p.Key("unknown-kid")
				p.Loaded()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for j := 0; j < 20; j++ {
			_ = p.fetch()
		}
	}()

	wg.Wait()
}
