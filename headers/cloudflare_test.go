package headers

import (
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
)

func TestCopyCloudFlareHeaders(t *testing.T) {
	original := config.Global.TrustedProxies
	t.Cleanup(func() { config.Global.TrustedProxies = original })
	config.Global.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")}

	ctx := fasthttp.RequestCtx{}
	req := fasthttp.Request{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("192.168.1.10")})

	ctx.Request.Header.SetBytesV("Cf-Connection-Ip", []byte("192.168.1.1, 10.0.0.1"))
	ctx.Request.Header.SetBytesV("X-Request-Id", []byte("123456"))
	ctx.Request.Header.SetBytesV("CfIndex", []byte("qwerty"))

	CopyCloudFlareHeaders(&ctx, &req)

	assert.Equal(t, "192.168.1.1, 10.0.0.1", string(req.Header.Peek("Cf-Original-Connection-Ip")))
	assert.Empty(t, req.Header.Peek("Cf-Connection-Ip"))
	assert.Empty(t, req.Header.Peek("X-Request-Id"))
	assert.Empty(t, req.Header.Peek("CfIndex"))
}

func TestCopyCloudFlareHeadersValidConnectingIPCopied(t *testing.T) {
	original := config.Global.TrustedProxies
	t.Cleanup(func() { config.Global.TrustedProxies = original })
	config.Global.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")}

	ctx := fasthttp.RequestCtx{}
	req := fasthttp.Request{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("192.168.1.10")})

	ctx.Request.Header.SetBytesV(CfConnectingIP, []byte("203.0.113.5"))
	ctx.Request.Header.SetBytesV("Cf-Index", []byte("qwerty"))

	CopyCloudFlareHeaders(&ctx, &req)

	assert.Equal(t, "203.0.113.5", string(req.Header.Peek("Cf-Original-Connecting-Ip")))
	assert.Equal(t, "qwerty", string(req.Header.Peek("Cf-Original-Index")))
}

func TestCopyCloudFlareHeadersMalformedConnectingIPNotCopiedOthersAre(t *testing.T) {
	original := config.Global.TrustedProxies
	t.Cleanup(func() { config.Global.TrustedProxies = original })
	config.Global.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")}

	for name, malformed := range map[string]string{
		"NotAnIP":      "not-an-ip",
		"CIDR":         "1.2.3.4/24",
		"WithPort":     "1.2.3.4:80",
		"ObsFolded":    "1.2.3.4\r\n ,9.9.9.9",
		"EmbeddedNull": "1.2.3\x004.5",
	} {
		t.Run(name, func(t *testing.T) {
			ctx := fasthttp.RequestCtx{}
			req := fasthttp.Request{}
			ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("192.168.1.10")})

			ctx.Request.Header.SetBytesV(CfConnectingIP, []byte(malformed))
			ctx.Request.Header.SetBytesV("Cf-Ray", []byte("abc123"))

			CopyCloudFlareHeaders(&ctx, &req)

			assert.Empty(t, req.Header.Peek("Cf-Original-Connecting-Ip"))
			assert.Equal(t, "abc123", string(req.Header.Peek("Cf-Original-Ray")))
		})
	}
}

func TestCopyCloudFlareHeadersZoneScopedConnectingIPNotCopied(t *testing.T) {
	original := config.Global.TrustedProxies
	t.Cleanup(func() { config.Global.TrustedProxies = original })
	config.Global.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")}

	ctx := fasthttp.RequestCtx{}
	req := fasthttp.Request{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("192.168.1.10")})

	ctx.Request.Header.SetBytesV(CfConnectingIP, []byte("fe80::1%eth0"))

	CopyCloudFlareHeaders(&ctx, &req)

	assert.Empty(t, req.Header.Peek("Cf-Original-Connecting-Ip"))
}

func TestCopyCloudFlareHeadersUntrustedPeerCopiesNothing(t *testing.T) {
	original := config.Global.TrustedProxies
	t.Cleanup(func() { config.Global.TrustedProxies = original })
	config.Global.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")}

	ctx := fasthttp.RequestCtx{}
	req := fasthttp.Request{}
	ctx.SetRemoteAddr(&net.TCPAddr{IP: net.ParseIP("203.0.113.10")})

	ctx.Request.Header.SetBytesV(CfConnectingIP, []byte("1.2.3.4"))
	ctx.Request.Header.SetBytesV("CfIndex", []byte("qwerty"))

	CopyCloudFlareHeaders(&ctx, &req)

	// A spoofed Cf-Connecting-IP from an untrusted peer must never reach the API as authoritative.
	assert.Empty(t, req.Header.Peek("Cf-Original-Connecting-IP"))
	assert.Empty(t, req.Header.Peek("Cf-Original-Index"))
	assert.Zero(t, len(req.Header.PeekKeys()))
}
