package captcha

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
)

const verifyUrl = "https://www.google.com/recaptcha/api/siteverify"

var client *fasthttp.Client

type VerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	Score       float32  `json:"score,omitempty"`
	Action      string   `json:"action,omitempty"`
	ErrorCodes  []string `json:"error-codes,omitempty"`
}

func init() {
	newClient()
}

func newClient() {
	if client != nil {
		return
	}

	client = &fasthttp.Client{
		ReadTimeout:              500 * time.Millisecond,
		WriteTimeout:             time.Second,
		MaxIdleConnDuration:      time.Hour,
		NoDefaultUserAgentHeader: true,
		Dial: (&fasthttp.TCPDialer{
			Concurrency:      4096,
			DNSCacheDuration: time.Hour,
		}).Dial,
	}
}

func Verify(ctx *fasthttp.RequestCtx) (bool, error) {
	clientIp := headers.GetClientIPFromContext(ctx)

	if config.Global.CaptchaSecret == "" {
		log.Printf("[%s] captcha secret empty, skipping verify", clientIp)
		return true, nil
	}

	challenge := ctx.Request.Header.Peek(headers.XCtCaptchaChallenge)
	if challenge == nil || string(challenge) == "" {
		log.Printf("[%s] captcha challenge empty", clientIp)
		return false, nil
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
	}()

	prepareGoogleReCaptchaVerifyRequest(req, challenge, clientIp)

	if err := client.Do(req, resp); err != nil {
		return false, fmt.Errorf("captcha verify request error: %w", err)
	}

	verifyResponse := VerifyResponse{}
	if err := json.Unmarshal(resp.Body(), &verifyResponse); err != nil {
		return false, fmt.Errorf("captcha verify response unexpected: %w", err)
	}

	if !verifyResponse.Success {
		log.Printf("[%s] captcha verify unsuccessfull: score %f, errors: %s", clientIp, verifyResponse.Score, strings.Join(verifyResponse.ErrorCodes, ", "))
		return false, nil
	}

	log.Printf("[%s] captcha verify: ok", clientIp)

	return true, nil
}

func prepareGoogleReCaptchaVerifyRequest(req *fasthttp.Request, challenge []byte, clientIp string) {
	req.SetRequestURI(verifyUrl)
	req.Header.SetMethod(fasthttp.MethodPost)
	req.Header.SetContentTypeBytes(headers.ContentTypeForm)
	req.PostArgs().Set("secret", config.Global.CaptchaSecret)
	req.PostArgs().Set("remoteip", clientIp)
	req.PostArgs().SetBytesV("response", challenge)
}
