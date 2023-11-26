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

// CopyCloudFlareHeaders keep original CloudFlare incoming headers for other services behind gateway
func CopyCloudFlareHeaders(ctx *fasthttp.RequestCtx, req *fasthttp.Request) {
	for _, key := range ctx.Request.Header.PeekKeys() {
		if !strings.HasPrefix(string(key), CloudFlareIncomingHeaderPrefix) {
			continue
		}

		if val := ctx.Request.Header.PeekBytes(key); val != nil {
			req.Header.SetBytesV(
				CloudFlareInternalHeaderPrefix+strings.TrimPrefix(string(key), CloudFlareIncomingHeaderPrefix),
				bytes.Clone(val),
			)
		}
	}
}
