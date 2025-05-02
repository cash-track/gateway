package captcha

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/http/retryhttp"
	"github.com/cash-track/gateway/traces"
)

const (
	googleApiReCaptchaVerifyUrl = "https://www.google.com/recaptcha/api/siteverify"
	googleApiReadTimeout        = 500 * time.Millisecond
	googleApiWriteTimeout       = time.Second
	googleApiRetryAttempts      = uint(2)
)

type GoogleReCaptchaProvider struct {
	client retryhttp.Client
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

func (r *googleReCaptchaVerifyResponse) GetOpenTelemetryAttributes() []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Bool("ct.captcha.success", r.Success),
		attribute.String("ct.captcha.challenge_ts", r.ChallengeTS),
		attribute.String("ct.captcha.hostname", r.Hostname),
		attribute.Float64("ct.captcha.score", float64(r.Score)),
		attribute.String("ct.captcha.action", r.Action),
		attribute.String("ct.captcha.error_codes", strings.Join(r.ErrorCodes, ",")),
	}
}

func NewGoogleReCaptchaProvider(httpClient retryhttp.Client, options config.Config) *GoogleReCaptchaProvider {
	httpClient.WithReadTimeout(googleApiReadTimeout)
	httpClient.WithWriteTimeout(googleApiWriteTimeout)
	httpClient.WithRetryAttempts(googleApiRetryAttempts)

	return &GoogleReCaptchaProvider{
		client: httpClient,
		secret: options.CaptchaSecret,
	}
}

func (p *GoogleReCaptchaProvider) Verify(ctx *fasthttp.RequestCtx) (bool, error) {
	clientIp := headers.GetClientIPFromContext(ctx)

	_, span := traces.GetTracer().Start(
		traces.FindParentContext(ctx),
		fmt.Sprintf("google recaptcha %s %s", ctx.Request.Header.Method(), ctx.URI().PathOriginal()),
		trace.WithAttributes(
			traces.MergeAttributes(
				traces.Attributes(attribute.String("http.request.real_ip", clientIp)),
				traces.RequestAttributes(&ctx.Request),
			)...,
		),
	)
	defer span.End()

	if p.secret == "" {
		span.SetStatus(codes.Ok, "disabled")
		log.Printf("[%s] captcha secret empty, skipping verify", clientIp)
		return true, nil
	}

	if string(ctx.Request.Header.Method()) == fasthttp.MethodOptions {
		span.SetStatus(codes.Ok, "unsupported method")
		return true, nil
	}

	challenge := ctx.Request.Header.Peek(headers.XCtCaptchaChallenge)
	if challenge == nil || string(challenge) == "" {
		span.SetStatus(codes.Error, "empty challenge")
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

	span.SetAttributes(traces.RequestAttributes(req)...)

	if err := p.client.Do(req, resp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "request error")
		return false, fmt.Errorf("captcha verify request error: %w", err)
	}

	span.SetAttributes(traces.ResponseAttributes(resp)...)

	verifyResp := googleReCaptchaVerifyResponse{}
	if err := json.Unmarshal(resp.Body(), &verifyResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "read body error")
		return false, fmt.Errorf("captcha verify response unexpected: %w", err)
	}

	span.SetAttributes(traces.AttributesGetter(&verifyResp)...)

	if !verifyResp.Success {
		log.Printf("[%s] captcha verify unsuccessfull: score %f, errors: %s", clientIp, verifyResp.Score, strings.Join(verifyResp.ErrorCodes, ", "))
		span.SetStatus(codes.Error, "validation failed")
		return false, nil
	}

	log.Printf("[%s] captcha verify: ok", clientIp)
	span.SetStatus(codes.Ok, "ok")

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
