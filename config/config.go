package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Address       string
	Compress      bool
	CaptchaSecret string

	GatewayUrl string
	ApiUrl     string
	ApiURI     *url.URL
	WebsiteUrl string
	WebAppUrl  string

	HttpsEnabled bool
	HttpsKey     string
	HttpsCrt     string

	CookieDomain string
	CookieSecure bool

	CorsAllowedOrigins map[string]bool

	DebugHttp bool

	CsrfEnabled     bool
	RedisConnection string
}

var Global Config

func (c *Config) Load() {
	c.Address = getEnv("GATEWAY_ADDRESS", ":80")
	c.Compress = getEnv("GATEWAY_COMPRESS", "true") == "true"
	c.DebugHttp = getEnv("DEBUG_HTTP", "") == "true"
	c.CaptchaSecret = getEnv("CAPTCHA_SECRET", "")

	c.ApiUrl = getEnv("API_URL", "")
	if u, err := url.Parse(c.ApiUrl); err != nil {
		panic(fmt.Sprintf("Unexpected API_URL: %s", c.ApiURI))
	} else {
		c.ApiURI = u
	}

	c.GatewayUrl = getEnv("GATEWAY_URL", "")
	c.WebsiteUrl = getEnv("WEBSITE_URL", "")
	c.WebAppUrl = getEnv("WEBAPP_URL", "")

	c.HttpsEnabled = getEnv("HTTPS_ENABLED", "") == "true"
	c.HttpsKey = getEnv("HTTPS_KEY", "")
	c.HttpsCrt = getEnv("HTTPS_CRT", "")

	c.CookieDomain = getCookieDomain(c.GatewayUrl)
	c.CookieSecure = getCookieSecure(c.GatewayUrl)

	c.CorsAllowedOrigins = getCorsAllowedOrigins(getEnv("CORS_ALLOWED_ORIGINS", ""))

	c.CsrfEnabled = getEnv("CSRF_ENABLED", "") == "true"
	c.RedisConnection = getEnv("REDIS_CONNECTION", "localhost:6379")
}

func getEnv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}

	return v
}

func getCookieDomain(url string) string {
	domain := strings.ReplaceAll(url, "http://", "")
	domain = strings.ReplaceAll(domain, "https://", "")
	domain = strings.ReplaceAll(domain, "/", "")

	if strings.Contains(domain, ":") {
		list := strings.Split(domain, ":")
		if len(list) > 0 {
			domain = list[0]
		}
	}

	return domain
}

func getCookieSecure(url string) bool {
	return strings.Contains(url, "https")
}

func getCorsAllowedOrigins(val string) map[string]bool {
	list := make(map[string]bool)

	for _, v := range strings.Split(val, ",") {
		list[strings.ToLower(v)] = true
	}

	return list
}
