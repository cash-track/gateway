package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigLoad(t *testing.T) {
	_ = os.Setenv("GATEWAY_ADDRESS", ":80")
	_ = os.Setenv("GATEWAY_COMPRESS", "true")
	_ = os.Setenv("DEBUG_HTTP", "false")
	_ = os.Setenv("API_URL", "http://api:80")
	_ = os.Setenv("GATEWAY_URL", "https://gateway.dev.cash-track.app:8081")
	_ = os.Setenv("HTTPS_ENABLED", "true")
	_ = os.Setenv("CORS_ALLOWED_ORIGINS", "https://My.dev.cash-track.app:3001,https://Dev.cash-track.app:3000")
	_ = os.Setenv("CSRF_ENABLED", "true")
	_ = os.Setenv("REDIS_CONNECTION", "redis:1234")

	config := &Config{}
	config.Load()

	assert.Equal(t, ":80", config.Address)
	assert.Equal(t, true, config.Compress)
	assert.Equal(t, false, config.DebugHttp)

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

	assert.Equal(t, true, config.CsrfEnabled)
	assert.Equal(t, "redis:1234", config.RedisConnection)
}

func TestConfigLoadUnexpectedApiUrl(t *testing.T) {
	_ = os.Setenv("API_URL", "://api")

	config := &Config{}

	assert.Panics(t, func() {
		config.Load()
	})
}
