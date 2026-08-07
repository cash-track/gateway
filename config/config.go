package config

import (
	"fmt"
	"log"
	"net/netip"
	"net/url"
	"os"
	"strings"
)

// RFC1918 + loopback: covers Traefik on a Docker bridge network out of the box.
const defaultTrustedProxies = "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.0/8,::1/128"

type Config struct {
	Address       string
	Compress      bool
	CaptchaSecret string
	GatewaySecret string

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

	// Peers allowed to set Cf-Connecting-IP (e.g. Traefik).
	TrustedProxies []netip.Prefix

	DebugHttp        bool
	TraceCaptureBody bool

	CsrfEnabled     bool
	RedisConnection string

	GitTag string
	GitSha string
}

var Global Config

func (c *Config) Load() {
	c.Address = getEnv("GATEWAY_ADDRESS", ":80")
	c.Compress = getEnv("GATEWAY_COMPRESS", "true") == "true"
	c.DebugHttp = getEnv("DEBUG_HTTP", "") == "true"
	c.TraceCaptureBody = getEnv("TRACE_CAPTURE_BODY", "true") == "true"
	c.CaptchaSecret = getEnv("CAPTCHA_SECRET", "")
	c.GatewaySecret = getEnv("GATEWAY_SECRET", "")

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
	c.TrustedProxies = getTrustedProxies(getEnv("TRUSTED_PROXIES", defaultTrustedProxies))

	c.CsrfEnabled = getEnv("CSRF_ENABLED", "") == "true"
	c.RedisConnection = getEnv("REDIS_CONNECTION", "localhost:6379")

	c.GitTag = getEnv("GIT_TAG", "")
	c.GitSha = getEnv("GIT_COMMIT", "")
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

// getTrustedProxies parses a comma-separated list of CIDRs and/or bare IPs.
// Bare IPs are normalised to a /32 or /128 prefix. Invalid entries are logged and skipped.
func getTrustedProxies(val string) []netip.Prefix {
	list := make([]netip.Prefix, 0)

	for _, v := range strings.Split(val, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}

		prefix, err := parseTrustedProxy(v)
		if err != nil {
			log.Printf("Skipping invalid TRUSTED_PROXIES entry %q: %s", v, err)

			continue
		}

		list = append(list, prefix)
	}

	return list
}

func parseTrustedProxy(v string) (netip.Prefix, error) {
	if prefix, err := netip.ParsePrefix(v); err == nil {
		return prefix, nil
	}

	addr, err := netip.ParseAddr(v)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("not a valid CIDR or IP: %w", err)
	}

	return netip.PrefixFrom(addr, addr.BitLen()), nil
}
