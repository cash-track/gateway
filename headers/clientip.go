package headers

import "github.com/valyala/fasthttp"

var clientIpUserValue = []byte("ClientIP")
var ipHeaders = []string{CfConnectingIP, XRealIp, XForwardedFor}

func GetClientIPFromContext(ctx *fasthttp.RequestCtx) string {
	if v, ok := ctx.UserValueBytes(clientIpUserValue).(string); ok {
		return v
	}

	return ctx.RemoteIP().String()
}

func findRealClientIP(ctx *fasthttp.RequestCtx) string {
	for _, h := range ipHeaders {
		if val := ctx.Request.Header.PeekAll(h); len(val) > 0 {
			for _, v := range val {
				if string(v) != "" {
					return string(v)
				}
			}
		}
	}

	return ctx.RemoteIP().String()
}
