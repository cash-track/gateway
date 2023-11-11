package headers

import (
	"bytes"

	"github.com/valyala/fasthttp"
)

const (
	Accept                        = "Accept"
	AcceptLanguage                = "Accept-Language"
	AccessControlAllowOrigin      = "Access-Control-Allow-Origin"
	AccessControlAllowMethods     = "Access-Control-Allow-Methods"
	AccessControlAllowHeaders     = "Access-Control-Allow-Headers"
	AccessControlAllowCredentials = "Access-Control-Allow-Credentials"
	AccessControlRequestMethod    = "Access-Control-Request-Method"
	AccessControlRequestHeaders   = "Access-Control-Request-Headers"
	AccessControlMaxAge           = "Access-Control-Max-Age"
	Authorization                 = "Authorization"
	CfConnectingIP                = "Cf-Connecting-IP"
	ContentType                   = "Content-Type"
	Origin                        = "Origin"
	Referer                       = "Referer"
	RetryAfter                    = "Retry-After"
	UserAgent                     = "User-Agent"
	Vary                          = "Vary"
	XCtCaptchaChallenge           = "X-Ct-Captcha-Challenge"
	XForwardedFor                 = "X-Forwarded-For"
	XRateLimit                    = "X-Ratelimit-Limit"
	XRateLimitRemaining           = "X-Ratelimit-Remaining"
	XRealIp                       = "X-Real-IP"
)

var (
	ContentTypeJson = []byte("application/json")
	ContentTypeForm = []byte("application/x-www-form-urlencoded")
)

func CopyFromRequest(ctx *fasthttp.RequestCtx, req *fasthttp.Request, headers []string) {
	for _, key := range headers {
		if val := ctx.Request.Header.PeekAll(key); val != nil {
			for _, v := range val {
				req.Header.SetBytesV(key, bytes.Clone(v))
			}
		}
	}
}

func CopyFromResponse(resp *fasthttp.Response, ctx *fasthttp.RequestCtx, headers []string) {
	for _, key := range headers {
		if val := resp.Header.PeekAll(key); val != nil {
			for _, v := range val {
				ctx.Response.Header.SetBytesV(key, bytes.Clone(v))
			}
		}
	}
}
