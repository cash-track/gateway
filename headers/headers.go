package headers

import (
	"bytes"
	"strings"

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
	multipleSep     = []byte(", ")
	ContentTypeJson = []byte("application/json")
	ContentTypeForm = []byte("application/x-www-form-urlencoded")

	// A list of headers which will always be overwritten if an attempt to write new value
	// occurs when other value already exists
	singleInstanceHeaders = map[string]bool{
		strings.ToLower(Authorization):       true,
		strings.ToLower(ContentType):         true,
		strings.ToLower(Origin):              true,
		strings.ToLower(Referer):             true,
		strings.ToLower(RetryAfter):          true,
		strings.ToLower(UserAgent):           true,
		strings.ToLower(XCtCaptchaChallenge): true,
		strings.ToLower(XRateLimit):          true,
		strings.ToLower(XRateLimitRemaining): true,
	}
)

func CopyFromRequest(ctx *fasthttp.RequestCtx, req *fasthttp.Request, headers []string) {
	for _, key := range headers {
		if v := ctx.Request.Header.PeekAll(key); len(v) > 0 {
			value := copyAll(v)
			_, single := singleInstanceHeaders[strings.ToLower(key)]
			if existing := req.Header.PeekAll(key); !single && len(existing) > 0 {
				value = append(existing, value...)
			}

			req.Header.SetBytesV(key, bytes.Join(value, multipleSep))
		}
	}
}

func CopyFromResponse(resp *fasthttp.Response, ctx *fasthttp.RequestCtx, headers []string) {
	for _, key := range headers {
		if v := resp.Header.PeekAll(key); len(v) > 0 {
			value := copyAll(v)
			_, single := singleInstanceHeaders[strings.ToLower(key)]
			if existing := ctx.Response.Header.PeekAll(key); !single && len(existing) > 0 {
				value = append(existing, value...)
			}

			ctx.Response.Header.SetBytesV(key, bytes.Join(value, multipleSep))
		}
	}
}

func copyAll(src [][]byte) [][]byte {
	value := make([][]byte, 0, len(src))
	for _, v := range src {
		value = append(value, bytes.Clone(v))
	}
	return value
}
