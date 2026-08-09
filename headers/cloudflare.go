package headers

import (
	"bytes"
	"strings"

	"github.com/valyala/fasthttp"
)

const (
	CloudFlareIncomingHeaderPrefix = "Cf-"
	CloudFlareInternalHeaderPrefix = "Cf-Original-"
)

// CopyCloudFlareHeaders keeps original CloudFlare incoming headers for other services behind gateway.
// Only copies from a trusted peer — otherwise Cf-* is attacker input, not real CloudFlare data.
func CopyCloudFlareHeaders(ctx *fasthttp.RequestCtx, req *fasthttp.Request) {
	if !isTrustedPeer(ctx) {
		return
	}

	for _, key := range ctx.Request.Header.PeekKeys() {
		if !strings.HasPrefix(string(key), CloudFlareIncomingHeaderPrefix) {
			continue
		}

		if val := ctx.Request.Header.PeekBytes(key); val != nil {
			// Cf-Connecting-IP keys the upstream rate limiter's bucket: drop it here rather than forward garbage.
			if strings.EqualFold(string(key), CfConnectingIP) {
				if _, ok := parseClientIP(val); !ok {
					continue
				}
			}

			req.Header.SetBytesV(
				CloudFlareInternalHeaderPrefix+strings.TrimPrefix(string(key), CloudFlareIncomingHeaderPrefix),
				bytes.Clone(val),
			)
		}
	}
}
