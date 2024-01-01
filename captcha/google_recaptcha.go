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
	"github.com/cash-track/gateway/http"
)

const (
	googleApiReCaptchaVerifyUrl = "https://www.google.com/recaptcha/api/siteverify"
	googleApiReadTimeout        = 500 * time.Millisecond
	googleApiWriteTimeout       = time.Second
)

type GoogleReCaptchaProvider struct {
	client http.Client
	secret string
}

type googleReCaptchaVerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	Score       float32  `json:"score,omitempty"`
	Action      string   `json:"action,omitempty"`
	ErrorCodes  []string `json:"error-codes,omitempty"`
}

func NewGoogleReCaptchaProvider(httpClient http.Client, options config.Config) *GoogleReCaptchaProvider {
	httpClient.WithReadTimeout(googleApiReadTimeout)
	httpClient.WithWriteTimeout(googleApiWriteTimeout)

	return &GoogleReCaptchaProvider{
		client: httpClient,
		secret: options.CaptchaSecret,
	}
}

func (p *GoogleReCaptchaProvider) Verify(ctx *fasthttp.RequestCtx) (bool, error) {
	clientIp := headers.GetClientIPFromContext(ctx)

	if p.secret == "" {
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

	p.buildReq(req, challenge, clientIp)

	if err := p.client.Do(req, resp); err != nil {
		return false, fmt.Errorf("captcha verify request error: %w", err)
	}

	verifyResp := googleReCaptchaVerifyResponse{}
	if err := json.Unmarshal(resp.Body(), &verifyResp); err != nil {
		return false, fmt.Errorf("captcha verify response unexpected: %w", err)
	}

	if !verifyResp.Success {
		log.Printf("[%s] captcha verify unsuccessfull: score %f, errors: %s", clientIp, verifyResp.Score, strings.Join(verifyResp.ErrorCodes, ", "))
		return false, nil
	}

	log.Printf("[%s] captcha verify: ok", clientIp)

	return true, nil
}

func (p *GoogleReCaptchaProvider) buildReq(req *fasthttp.Request, challenge []byte, clientIp string) {
	req.SetRequestURI(googleApiReCaptchaVerifyUrl)
	req.Header.SetMethod(fasthttp.MethodPost)
	req.Header.SetContentTypeBytes(headers.ContentTypeForm)
	req.PostArgs().Set("secret", p.secret)
	req.PostArgs().Set("remoteip", clientIp)
	req.PostArgs().SetBytesV("response", challenge)
}
