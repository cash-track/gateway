package config

import (
	"net/netip"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigLoad(t *testing.T) {
	_ = os.Setenv("GATEWAY_ADDRESS", ":80")
	_ = os.Setenv("GATEWAY_COMPRESS", "true")
	_ = os.Setenv("DEBUG_HTTP", "false")
	_ = os.Setenv("TRACE_CAPTURE_BODY", "false")
	_ = os.Setenv("API_URL", "http://api:80")
	_ = os.Setenv("GATEWAY_URL", "https://gateway.dev.cash-track.app:8081")
	_ = os.Setenv("HTTPS_ENABLED", "true")
	_ = os.Setenv("CORS_ALLOWED_ORIGINS", "https://My.dev.cash-track.app:3001,https://Dev.cash-track.app:3000")
	_ = os.Setenv("CSRF_ENABLED", "true")
	_ = os.Setenv("REDIS_CONNECTION", "redis:1234")
	_ = os.Setenv("GIT_TAG", "v1.2.3")
	_ = os.Setenv("GIT_COMMIT", "abc123def456")
	t.Setenv("TRUSTED_PROXIES", "")

	config := &Config{}
	config.Load()

	assert.Equal(t, ":80", config.Address)
	assert.Equal(t, true, config.Compress)
	assert.Equal(t, false, config.DebugHttp)
	assert.Equal(t, false, config.TraceCaptureBody)

	assert.NotNil(t, config.ApiURI)
	assert.Equal(t, "http", config.ApiURI.Scheme)
	assert.Equal(t, "api:80", config.ApiURI.Host)
	assert.Equal(t, "", config.ApiURI.Path)

	assert.Equal(t, "https://gateway.dev.cash-track.app:8081", config.GatewayUrl)
	assert.Equal(t, true, config.HttpsEnabled)

	assert.Equal(t, "gateway.dev.cash-track.app", config.CookieDomain)
	assert.Equal(t, true, config.CookieSecure)

	assert.NotNil(t, config.CorsAllowedOrigins)
	assert.Len(t, config.CorsAllowedOrigins, 2)

	_, ok := config.CorsAllowedOrigins["https://my.dev.cash-track.app:3001"]
	assert.Equal(t, true, ok)

	_, ok = config.CorsAllowedOrigins["https://dev.cash-track.app:3000"]
	assert.Equal(t, true, ok)

	assert.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	}, config.TrustedProxies)

	assert.Equal(t, true, config.CsrfEnabled)
	assert.Equal(t, "redis:1234", config.RedisConnection)

	assert.Equal(t, "v1.2.3", config.GitTag)
	assert.Equal(t, "abc123def456", config.GitSha)
}

func TestConfigLoadGitInfoDefaultsEmpty(t *testing.T) {
	_ = os.Setenv("API_URL", "http://api:80")
	t.Setenv("GIT_TAG", "")
	t.Setenv("GIT_COMMIT", "")

	config := &Config{}
	config.Load()

	// No default is fabricated when GIT_TAG/GIT_COMMIT are unset (e.g. local `make run`).
	assert.Equal(t, "", config.GitTag)
	assert.Equal(t, "", config.GitSha)
}

func TestConfigLoadUnexpectedApiUrl(t *testing.T) {
	_ = os.Setenv("API_URL", "://api")

	config := &Config{}

	assert.Panics(t, func() {
		config.Load()
	})
}

func TestConfigLoadTrustedProxiesCustom(t *testing.T) {
	_ = os.Setenv("API_URL", "http://api:80")
	t.Setenv("TRUSTED_PROXIES", "10.10.0.0/16,,172.20.0.5, not-a-cidr, ::1,")

	config := &Config{}

	assert.NotPanics(t, func() {
		config.Load()
	})

	// bare IPs are normalised to /32 or /128, and the invalid entry is skipped, not fatal.
	assert.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("10.10.0.0/16"),
		netip.MustParsePrefix("172.20.0.5/32"),
		netip.MustParsePrefix("::1/128"),
	}, config.TrustedProxies)
}

func TestConfigLoadTrustedProxiesEmptyUsesDefault(t *testing.T) {
	_ = os.Setenv("API_URL", "http://api:80")
	t.Setenv("TRUSTED_PROXIES", "")

	config := &Config{}
	config.Load()

	assert.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	}, config.TrustedProxies)
}

func TestConfigLoadTrustedProxiesAllInvalidYieldsEmptyList(t *testing.T) {
	_ = os.Setenv("API_URL", "http://api:80")
	t.Setenv("TRUSTED_PROXIES", "garbage, also-garbage")

	config := &Config{}
	config.Load()

	assert.Empty(t, config.TrustedProxies)
}

func TestConfigLoadGatewaySecret(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "unset defaults to empty", env: "", want: ""},
		{name: "set is loaded verbatim", env: "shared-secret-value", want: "shared-secret-value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv("API_URL", "http://api:80")
			t.Setenv("GATEWAY_SECRET", tt.env)

			config := &Config{}
			config.Load()

			assert.Equal(t, tt.want, config.GatewaySecret)
		})
	}
}
