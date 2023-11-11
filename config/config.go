package config

import (
	"os"
	"strings"
)

type Config struct {
	Address       string
	Compress      bool
	CaptchaSecret string

	GatewayUrl string
	ApiUrl     string
	WebsiteUrl string
	WebAppUrl  string

	HttpsEnabled bool
	HttpsKey     string
	HttpsCrt     string

	CookieDomain string
	CookieSecure bool

	CorsAllowedOrigins map[string]bool

	DebugHttp bool
}

var Global Config

func (c *Config) Load() {
	c.Address = getEnv("GATEWAY_ADDRESS", ":8080")
	c.Compress = getEnv("GATEWAY_COMPRESS", "true") == "true"
	c.DebugHttp = getEnv("DEBUG_HTTP", "") == "true"
	c.CaptchaSecret = getEnv("CAPTCHA_SECRET", "")

	c.GatewayUrl = getEnv("GATEWAY_URL", "")
	c.ApiUrl = getEnv("API_URL", "")
	c.WebsiteUrl = getEnv("WEBSITE_URL", "")
	c.WebAppUrl = getEnv("WEBAPP_URL", "")

	if strings.HasPrefix(c.GatewayUrl, "https") {
		c.HttpsEnabled = true
		c.HttpsKey = getEnv("HTTPS_KEY", "")
		c.HttpsCrt = getEnv("HTTPS_CRT", "")
	}

	c.CookieDomain = getCookieDomain(c.GatewayUrl)
	c.CookieSecure = getCookieSecure(c.GatewayUrl)

	c.CorsAllowedOrigins = getCorsAllowedOrigins(getEnv("CORS_ALLOWED_ORIGINS", ""))
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

	if strings.Contains(domain, "localhost") {
		domain = "localhost"
	} else {
		domain += "."
	}

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
