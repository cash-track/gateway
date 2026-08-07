package headers

import (
	"net/netip"
	"strings"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
)

var clientIpUserValue = []byte("ClientIP")

// Only Cf-Connecting-IP: it's the only header here Cloudflare always sets and overwrites itself.
var ipHeaders = []string{CfConnectingIP}

func GetClientIPFromContext(ctx *fasthttp.RequestCtx) string {
	if v, ok := ctx.UserValueBytes(clientIpUserValue).(string); ok {
		return v
	}

	return ctx.RemoteIP().String()
}

// findRealClientIP trusts the IP headers only when the direct peer is a configured trusted proxy.
func findRealClientIP(ctx *fasthttp.RequestCtx) string {
	if isTrustedPeer(ctx) {
		for _, h := range ipHeaders {
			for _, line := range ctx.Request.Header.PeekAll(h) {
				if addr, ok := parseClientIP(line); ok {
					return addr.String()
				}
			}
		}
	}

	return ctx.RemoteIP().String()
}

// parseClientIP trims and validates a header value as a bare IP; zone-scoped addresses (fe80::1%eth0) are rejected.
func parseClientIP(raw []byte) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(string(raw)))
	if err != nil || addr.Zone() != "" {
		return netip.Addr{}, false
	}

	return addr, true
}

// isTrustedPeer reports whether the direct connection peer is a configured trusted proxy.
func isTrustedPeer(ctx *fasthttp.RequestCtx) bool {
	peer, ok := netip.AddrFromSlice(ctx.RemoteIP())

	return ok && isTrustedProxy(peer.Unmap())
}

func isTrustedProxy(addr netip.Addr) bool {
	for _, prefix := range config.Global.TrustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}

	return false
}
