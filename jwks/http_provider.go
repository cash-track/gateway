package jwks

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/http/retryhttp"
	"github.com/cash-track/gateway/logger"
)

const (
	ServiceId = "JWKS"

	httpReadTimeout  = 5 * time.Second
	httpWriteTimeout = 5 * time.Second

	// Rate-limits the unknown-kid refresh, so a burst of such tokens can't hammer the API.
	onDemandRefreshMinInterval = time.Minute
)

// Vars, not consts, so tests can shrink them.
var (
	bootstrapBackoffMin = time.Second
	bootstrapBackoffMax = 30 * time.Second

	// On-demand refresh only reacts to a kid appearing, never to one revoked at the API.
	// This bounds how long a rotated-out key stays trusted here.
	periodicRefreshInterval = 15 * time.Minute
)

// Unversioned: must NOT go through the "/v1" prefixing helper used for forwarded requests.
var jwksURI = []byte("/.well-known/jwks.json")

type jwkKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

// HttpProvider fetches the API's RFC 7517 JWK Set over HTTP and holds the parsed RSA
// public keys in memory, indexed by kid. Safe for concurrent use.
type HttpProvider struct {
	http   retryhttp.Client
	config config.Config

	// Immutable map, swapped wholesale on each successful fetch, so reads take no lock.
	keys atomic.Pointer[map[string]*rsa.PublicKey]

	// Guards lastRefreshAttempt only, to rate-limit on-demand refreshes.
	refreshMu          sync.Mutex
	lastRefreshAttempt time.Time
}

func NewHttp(httpClient retryhttp.Client, cfg config.Config) *HttpProvider {
	httpClient.WithReadTimeout(httpReadTimeout)
	httpClient.WithWriteTimeout(httpWriteTimeout)

	p := &HttpProvider{
		http:   httpClient,
		config: cfg,
	}

	empty := map[string]*rsa.PublicKey{}
	p.keys.Store(&empty)

	return p
}

// Start loads the JWKS in the background and returns immediately; call once from main.
// A failed or empty initial fetch is non-fatal - the gateway serves while it retries.
func (p *HttpProvider) Start(ctx context.Context) {
	go p.run(ctx)
}

// Key returns the RSA public key for kid, refreshing once (rate-limited) if kid is unknown.
func (p *HttpProvider) Key(kid string) (*rsa.PublicKey, bool) {
	if key, ok := p.lookup(kid); ok {
		return key, true
	}

	p.refreshRateLimited()

	return p.lookup(kid)
}

// Loaded reports whether any key material is currently held in memory.
func (p *HttpProvider) Loaded() bool {
	return len(*p.keys.Load()) > 0
}

// run drives the provider for the lifetime of ctx: bootstrap, then periodic refresh.
func (p *HttpProvider) run(ctx context.Context) {
	if !p.bootstrap(ctx) {
		return
	}

	p.periodicRefresh(ctx)
}

// bootstrap retries with bounded backoff until a key is loaded or ctx is cancelled.
func (p *HttpProvider) bootstrap(ctx context.Context) bool {
	backoff := bootstrapBackoffMin

	for {
		err := p.fetch()
		if err == nil && p.Loaded() {
			slog.Info("jwks keys loaded")

			return true
		}

		if err != nil {
			slog.Warn("jwks initial fetch failed, retrying in background", "error", err, "retry_in", backoff.String())
		} else {
			slog.Warn("jwks fetch returned no usable keys, retrying in background", "retry_in", backoff.String())
		}

		select {
		case <-ctx.Done():
			return false
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > bootstrapBackoffMax {
			backoff = bootstrapBackoffMax
		}
	}
}

// periodicRefresh re-fetches every periodicRefreshInterval until ctx ends. A failed tick
// is logged and retried on the next one.
func (p *HttpProvider) periodicRefresh(ctx context.Context) {
	ticker := time.NewTicker(periodicRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.fetch(); err != nil {
				slog.Warn("jwks periodic refresh failed", "error", err)
			}
		}
	}
}

func (p *HttpProvider) lookup(kid string) (*rsa.PublicKey, bool) {
	m := *p.keys.Load()
	key, ok := m[kid]

	return key, ok
}

func (p *HttpProvider) refreshRateLimited() {
	p.refreshMu.Lock()
	if time.Since(p.lastRefreshAttempt) < onDemandRefreshMinInterval {
		p.refreshMu.Unlock()

		return
	}
	p.lastRefreshAttempt = time.Now()
	p.refreshMu.Unlock()

	if err := p.fetch(); err != nil {
		slog.Warn("jwks on-demand refresh failed", "error", err)
	}
}

// fetch replaces the in-memory key map with the API's current JWK Set. An error, or a
// response with no usable RSA keys, leaves existing keys untouched: a loaded key set must
// never regress to empty, which would flip router/csrf to fail-open with no way back
// (only bootstrap and an unknown-kid lookup re-fetch, and the latter needs Loaded()).
func (p *HttpProvider) fetch() error {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.Header.SetMethod(fasthttp.MethodGet)
	p.setRequestURI(req.URI())
	req.Header.SetContentTypeBytes(headers.ContentTypeJson)
	req.Header.SetBytesV(headers.Accept, headers.ContentTypeJson)
	headers.WriteGatewayVersion(&req.Header, p.config.GitTag, p.config.GitSha)
	headers.WriteGatewaySecret(&req.Header, p.config.GatewaySecret)

	logger.DebugRequest(req, ServiceId)

	if err := p.http.Do(req, resp); err != nil {
		return fmt.Errorf("jwks fetch request error: %w", err)
	}

	logger.DebugResponse(resp, ServiceId)

	if resp.StatusCode() != fasthttp.StatusOK {
		return fmt.Errorf("jwks fetch failed [%d]", resp.StatusCode())
	}

	var body jwksResponse
	if err := json.Unmarshal(resp.Body(), &body); err != nil {
		return fmt.Errorf("jwks response unmarshal error: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(body.Keys))
	for _, k := range body.Keys {
		if k.Kty != "RSA" {
			continue
		}

		pubKey, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			slog.Warn("jwks entry skipped: invalid key material", "kid", k.Kid, "error", err)

			continue
		}

		keys[k.Kid] = pubKey
	}

	if len(keys) == 0 {
		if p.Loaded() {
			slog.Warn("jwks fetch returned no usable RSA keys, keeping previously loaded key set")
		} else {
			slog.Warn("jwks fetch returned no usable RSA keys")
		}

		return nil
	}

	p.keys.Store(&keys)

	return nil
}

func (p *HttpProvider) setRequestURI(dest *fasthttp.URI) {
	_ = dest.Parse([]byte(p.config.ApiUrl), nil)
	dest.SetScheme(p.config.ApiURI.Scheme)
	dest.SetHost(p.config.ApiURI.Host)
	dest.SetPathBytes(jwksURI)
}

// parseRSAPublicKey builds a key from a JWK's base64url "n" and "e", per RFC 7518 6.3.1.
func parseRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("invalid modulus (n): %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("invalid exponent (e): %w", err)
	}

	if len(nBytes) == 0 || len(eBytes) == 0 {
		return nil, fmt.Errorf("empty modulus or exponent")
	}

	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() || e.Int64() <= 0 || e.Int64() > math.MaxInt32 {
		return nil, fmt.Errorf("exponent out of range")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(e.Int64()),
	}, nil
}
