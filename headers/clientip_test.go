package headers

import (
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
)

func TestGetClientIPFromContext(t *testing.T) {
	ctx := fasthttp.RequestCtx{}

	clientIp := GetClientIPFromContext(&ctx)
	assert.Equal(t, "0.0.0.0", clientIp)

	ctx.SetUserValueBytes(clientIpUserValue, "192.168.1.3")

	clientIp = GetClientIPFromContext(&ctx)
	assert.Equal(t, "192.168.1.3", clientIp)
}

func TestFindRealClientIP(t *testing.T) {
	trustedProxies := []netip.Prefix{
		netip.MustParsePrefix("192.168.1.0/24"),
		netip.MustParsePrefix("::1/128"),
	}

	for name, test := range map[string]struct {
		RemoteAddr     string
		TrustedProxies []netip.Prefix
		Headers        map[string]string
		ExpectedIP     string
	}{
		"TrustedPeerCloudFlareHeaderWins": {
			RemoteAddr:     "192.168.1.10",
			TrustedProxies: trustedProxies,
			Headers: map[string]string{
				CfConnectingIP: "203.0.113.5",
				XRealIp:        "203.0.113.6",
				XForwardedFor:  "203.0.113.7",
			},
			ExpectedIP: "203.0.113.5",
		},
		"UntrustedPeerSpoofedCloudFlareHeaderIgnored": {
			RemoteAddr:     "10.0.0.5",
			TrustedProxies: trustedProxies,
			Headers: map[string]string{
				CfConnectingIP: "203.0.113.5",
			},
			ExpectedIP: "10.0.0.5",
		},
		"TrustedPeerXRealIPAloneIsIgnoredFallsBackToRemoteIP": {
			// X-Real-IP has no authoritative writer here: never trusted, even from a trusted peer.
			RemoteAddr:     "192.168.1.10",
			TrustedProxies: trustedProxies,
			Headers: map[string]string{
				CfConnectingIP: "",
				XRealIp:        "203.0.113.6",
				XForwardedFor:  "203.0.113.7",
			},
			ExpectedIP: "192.168.1.10",
		},
		"TrustedPeerForwardedForAloneIsIgnoredFallsBackToRemoteIP": {
			// X-Forwarded-For alone is never enough: it's not in ipHeaders at all.
			RemoteAddr:     "192.168.1.10",
			TrustedProxies: trustedProxies,
			Headers: map[string]string{
				CfConnectingIP: "",
				XForwardedFor:  "203.0.113.7",
			},
			ExpectedIP: "192.168.1.10",
		},
		"TrustedPeerForwardedForAppendChainNeverYieldsAttackerEntry": {
			// Real chain shape is <attacker>, <real client>, <cf edge> — must not return "6.6.6.6".
			RemoteAddr:     "192.168.1.10",
			TrustedProxies: trustedProxies,
			Headers: map[string]string{
				XForwardedFor: "6.6.6.6, 203.0.113.9, 172.71.1.1",
			},
			ExpectedIP: "192.168.1.10",
		},
		"TrustedPeerMalformedCloudFlareFallsThroughToRemoteIP": {
			// A malformed Cf-Connecting-IP falls straight to RemoteIP; other headers are noise, not a fallback.
			RemoteAddr:     "192.168.1.10",
			TrustedProxies: trustedProxies,
			Headers: map[string]string{
				CfConnectingIP: "not-an-ip",
				XRealIp:        "203.0.113.6",
				XForwardedFor:  "still, not, an, ip",
			},
			ExpectedIP: "192.168.1.10",
		},
		"TrustedPeerZoneScopedIPv6HeaderRejectedFallsBackToRemoteIP": {
			// A zone identifier is never meaningful for a public client IP; must not leak into the result.
			RemoteAddr:     "::1",
			TrustedProxies: trustedProxies,
			Headers: map[string]string{
				CfConnectingIP: "fe80::1%eth0",
			},
			ExpectedIP: "::1",
		},
		"TrustedIPv6PeerIPv6HeaderValue": {
			RemoteAddr:     "::1",
			TrustedProxies: trustedProxies,
			Headers: map[string]string{
				CfConnectingIP: "2001:db8::1",
			},
			ExpectedIP: "2001:db8::1",
		},
		"TrustedPeerEmptyHeadersFallsThroughToRemoteIP": {
			RemoteAddr:     "192.168.1.10",
			TrustedProxies: trustedProxies,
			Headers:        map[string]string{},
			ExpectedIP:     "192.168.1.10",
		},
		"NoTrustedProxiesConfiguredHeaderIgnored": {
			RemoteAddr:     "192.168.1.10",
			TrustedProxies: nil,
			Headers: map[string]string{
				CfConnectingIP: "203.0.113.5",
			},
			ExpectedIP: "192.168.1.10",
		},
		"DefaultUnsetRemoteAddrNoHeaders": {
			// RemoteAddr left empty: skip SetRemoteAddr, exercising fasthttp's own zero value.
			ExpectedIP: "0.0.0.0",
		},
	} {
		t.Run(name, func(t *testing.T) {
			original := config.Global.TrustedProxies
			config.Global.TrustedProxies = test.TrustedProxies
			defer func() { config.Global.TrustedProxies = original }()

			ctx := fasthttp.RequestCtx{}
			if test.RemoteAddr != "" {
				ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP(test.RemoteAddr)})
			}

			for key, value := range test.Headers {
				ctx.Request.Header.Set(key, value)
			}

			clientIp := findRealClientIP(&ctx)
			assert.Equal(t, test.ExpectedIP, clientIp)
		})
	}
}
