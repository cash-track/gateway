package api

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/mocks"
	"github.com/cash-track/gateway/service/api"
)

// setTestLogger redirects slog.Default() to a buffer, restored on cleanup.
func setTestLogger(t *testing.T) *bytes.Buffer {
	t.Helper()

	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))

	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	return &output
}

func TestAuthSetHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	tomorrow := time.Now().Add(time.Hour * 24).Format(time.RFC3339)

	c.EXPECT().Verify(gomock.Any()).Return(true, nil)
	s.EXPECT().ForwardRequest(gomock.Any(), nil).DoAndReturn(func(ctx *fasthttp.RequestCtx, body []byte) error {
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.SetBodyString(fmt.Sprintf(`{"accessToken":"new_access_token","refreshTokenExpiredAt":"%s"}`, tomorrow))
		return nil
	})

	h.AuthSetHandler(&ctx)

	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), "new_access_token")
}

func TestAuthSetHandlerCaptchaFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	c.EXPECT().Verify(gomock.Any()).Return(false, nil)

	h.AuthSetHandler(&ctx)

	assert.Equal(t, fasthttp.StatusBadRequest, ctx.Response.StatusCode())
}

func TestAuthSetHandlerCaptchaError(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	c.EXPECT().Verify(gomock.Any()).Return(false, fmt.Errorf("captcha api down"))

	h.AuthSetHandler(&ctx)

	assert.Equal(t, fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
}

func TestAuthSetHandlerLoginError(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	c.EXPECT().Verify(gomock.Any()).Return(true, nil)
	s.EXPECT().ForwardRequest(gomock.Any(), nil).DoAndReturn(func(ctx *fasthttp.RequestCtx, body []byte) error {
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		ctx.Response.SetBodyString(`{"accessToken":"new_access_token"`)
		return nil
	})

	h.AuthSetHandler(&ctx)

	assert.Equal(t, fasthttp.StatusBadGateway, ctx.Response.StatusCode())
}

func TestAuthResetHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.SetCookie(cookie.RefreshTokenCookieName, "refresh_token_test")

	body := []byte(`{"refreshToken":"refresh_token_test"}`)

	s.EXPECT().ForwardRequest(gomock.Any(), body).Return(nil)

	h.AuthResetHandler(&ctx)

	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), fmt.Sprintf("%s=;", cookie.AccessTokenCookieName))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), fmt.Sprintf("%s=;", cookie.RefreshTokenCookieName))
}

// Logout stays effective when the forward fails, so an open breaker does not turn every
// logout into a 503 for the whole timeout window.
func TestAuthResetHandlerForwardError(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.SetCookie(cookie.RefreshTokenCookieName, "refresh_token_test")

	body := []byte(`{"refreshToken":"refresh_token_test"}`)

	s.EXPECT().ForwardRequest(gomock.Any(), body).Return(fmt.Errorf("wrapped: %w", api.ErrCircuitOpen))

	h.AuthResetHandler(&ctx)

	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Empty(t, ctx.Response.Header.Peek(headers.RetryAfter))
	assert.JSONEq(t, `{"redirectUrl":""}`, string(ctx.Response.Body()))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), fmt.Sprintf("%s=;", cookie.AccessTokenCookieName))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), fmt.Sprintf("%s=;", cookie.RefreshTokenCookieName))
}

// ForwardRequest returns nil on any completed round-trip and copies the backend's raw
// status and headers onto ctx, so a backend 4xx/5xx must not leak into this always-200
// endpoint.
func TestAuthResetHandlerBackendErrorStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.SetCookie(cookie.RefreshTokenCookieName, "refresh_token_test")

	body := []byte(`{"refreshToken":"refresh_token_test"}`)

	s.EXPECT().ForwardRequest(gomock.Any(), body).DoAndReturn(func(ctx *fasthttp.RequestCtx, body []byte) error {
		ctx.Response.SetStatusCode(fasthttp.StatusUnauthorized)
		ctx.Response.Header.Set(headers.RetryAfter, "60")
		ctx.Response.Header.SetContentType("text/html; charset=utf-8")

		return nil
	})

	h.AuthResetHandler(&ctx)

	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Equal(t, string(headers.ContentTypeJson), string(ctx.Response.Header.ContentType()))
	assert.Empty(t, ctx.Response.Header.Peek(headers.RetryAfter))
	assert.JSONEq(t, `{"redirectUrl":""}`, string(ctx.Response.Body()))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), fmt.Sprintf("%s=;", cookie.AccessTokenCookieName))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), fmt.Sprintf("%s=;", cookie.RefreshTokenCookieName))
}

// Mocks only the HTTP transport so the real CopyFromResponse runs: a backend 429 carries
// Retry-After, X-Ratelimit-* and its own Content-Type, none of which may leak here.
func TestAuthResetHandlerBackendRateLimited(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := mocks.NewCaptchaProviderMock(ctrl)
	httpClient := mocks.NewHttpRetryClientMock(ctrl)
	httpClient.EXPECT().WithReadTimeout(gomock.Any())
	httpClient.EXPECT().WithWriteTimeout(gomock.Any())
	httpClient.EXPECT().WithRetryAttempts(gomock.Any())
	httpClient.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusTooManyRequests)
		resp.Header.Set(headers.RetryAfter, "42")
		resp.Header.Set(headers.XRateLimit, "60")
		resp.Header.Set(headers.XRateLimitRemaining, "0")
		resp.Header.SetContentType("application/json")
		resp.SetBodyString(`{"message":"too many requests"}`)

		return nil
	})

	apiUrl, _ := url.Parse("https://backend.test.com")
	svc := api.NewHttp(httpClient, config.Config{ApiURI: apiUrl}, nil, api.NewBreaker())
	h := NewHttp(config.Config{}, svc, c, &mockCSRFSeeder{})

	uri := &fasthttp.URI{}
	_ = uri.Parse(nil, []byte("https://gateway.test.com/api/auth/logout"))

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.SetCookie(cookie.RefreshTokenCookieName, "refresh_token_test")
	ctx.Request.SetURI(uri)

	// The real middleware applies the default Content-Type after the inner handler, which
	// is where a Response.Reset() can regress to text/plain or no Content-Type at all.
	headers.Handler(func(ctx *fasthttp.RequestCtx) {
		h.AuthResetHandler(ctx)
	})(&ctx)

	assert.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	assert.Equal(t, string(headers.ContentTypeJson), string(ctx.Response.Header.ContentType()))
	assert.Empty(t, ctx.Response.Header.Peek(headers.RetryAfter))
	assert.Empty(t, ctx.Response.Header.Peek(headers.XRateLimit))
	assert.Empty(t, ctx.Response.Header.Peek(headers.XRateLimitRemaining))
	assert.JSONEq(t, `{"redirectUrl":""}`, string(ctx.Response.Body()))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), fmt.Sprintf("%s=;", cookie.AccessTokenCookieName))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), fmt.Sprintf("%s=;", cookie.RefreshTokenCookieName))
}

func TestFullForwardedHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	s.EXPECT().ForwardRequest(gomock.Any(), nil).DoAndReturn(func(ctx *fasthttp.RequestCtx, body []byte) error {
		assert.Equal(t, fasthttp.MethodPost, string(ctx.Request.Header.Method()))
		assert.Equal(t, "Value", string(ctx.Request.Header.Peek("Test")))
		return nil
	})

	h.FullForwardedHandler(&ctx)
}

func TestFullForwardedHandlerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	s.EXPECT().ForwardRequest(gomock.Any(), nil).Return(fmt.Errorf("broken pipe"))

	h.FullForwardedHandler(&ctx)

	assert.Equal(t, fasthttp.StatusBadGateway, ctx.Response.StatusCode())
}

// Circuit-open is an expected degraded-mode response (has its own Retry-After), so it
// must log at Warn, not Error.
func TestWriteForwardErrorLogsAtWarnOnCircuitOpen(t *testing.T) {
	output := setTestLogger(t)

	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetRequestURI("/wallets")

	s.EXPECT().ForwardRequest(gomock.Any(), nil).Return(fmt.Errorf("wrapped: %w", api.ErrCircuitOpen))

	h.FullForwardedHandler(&ctx)

	logs := output.String()

	assert.Contains(t, logs, `"level":"WARN"`)
	assert.Contains(t, logs, "forward request rejected: circuit breaker open")
	assert.Contains(t, logs, `"method":"POST"`)
	assert.Contains(t, logs, `"path":"/wallets"`)
}

// A non-circuit-breaker transport failure is unexpected and must log at Error.
func TestWriteForwardErrorLogsAtErrorOnGenericFailure(t *testing.T) {
	output := setTestLogger(t)

	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetRequestURI("/wallets")

	s.EXPECT().ForwardRequest(gomock.Any(), nil).Return(fmt.Errorf("broken pipe"))

	h.FullForwardedHandler(&ctx)

	logs := output.String()

	assert.Contains(t, logs, `"level":"ERROR"`)
	assert.Contains(t, logs, "forward request failed")
	assert.Contains(t, logs, `"method":"POST"`)
	assert.Contains(t, logs, `"path":"/wallets"`)
}

func TestFullForwardedHandlerCircuitOpen(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)

	s.EXPECT().ForwardRequest(gomock.Any(), nil).Return(fmt.Errorf("wrapped: %w", api.ErrCircuitOpen))

	h.FullForwardedHandler(&ctx)

	assert.Equal(t, fasthttp.StatusServiceUnavailable, ctx.Response.StatusCode())
	assert.Equal(t, strconv.Itoa(api.RetryAfterSeconds), string(ctx.Response.Header.Peek(headers.RetryAfter)))
}

func TestFullForwardedHandlerWithBodyCircuitOpen(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)

	s.EXPECT().ForwardRequest(gomock.Any(), []byte(`{"test":"123"}`)).Return(fmt.Errorf("wrapped: %w", api.ErrCircuitOpen))

	h.FullForwardedHandlerWithBody(&ctx, map[string]string{"test": "123"})

	assert.Equal(t, fasthttp.StatusServiceUnavailable, ctx.Response.StatusCode())
	assert.Equal(t, strconv.Itoa(api.RetryAfterSeconds), string(ctx.Response.Header.Peek(headers.RetryAfter)))
}

func TestFullForwardedHandlerRestrictedMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodConnect)
	ctx.Request.Header.Set("Test", "Value")

	h.FullForwardedHandler(&ctx)

	assert.Equal(t, fasthttp.StatusBadRequest, ctx.Response.StatusCode())
}

func TestFullForwardedHandlerWithBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	body := cookie.Auth{
		AccessToken: "123",
	}
	bodyJson := []byte(`{"accessToken":"123"}`)

	s.EXPECT().ForwardRequest(gomock.Any(), bodyJson).DoAndReturn(func(ctx *fasthttp.RequestCtx, body []byte) error {
		assert.Equal(t, fasthttp.MethodPost, string(ctx.Request.Header.Method()))
		assert.Equal(t, "Value", string(ctx.Request.Header.Peek("Test")))
		return nil
	})

	h.FullForwardedHandlerWithBody(&ctx, body)
}

func TestFullForwardedHandlerWithBodyError(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	s.EXPECT().ForwardRequest(gomock.Any(), []byte(`{"test":"123"}`)).Return(fmt.Errorf("broken pipe"))

	h.FullForwardedHandlerWithBody(&ctx, map[string]string{"test": "123"})

	assert.Equal(t, fasthttp.StatusBadGateway, ctx.Response.StatusCode())
}

func TestFullForwardedHandlerWithBodyJsonError(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.Header.Set("Test", "Value")

	var i complex128
	h.FullForwardedHandlerWithBody(&ctx, i)

	assert.Equal(t, fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
}

func TestFullForwardedHandlerWithBodyRestrictedMethod(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodConnect)
	ctx.Request.Header.Set("Test", "Value")

	h.FullForwardedHandlerWithBody(&ctx, nil)

	assert.Equal(t, fasthttp.StatusBadRequest, ctx.Response.StatusCode())
}

func TestHealthcheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mocks.NewApiServiceMock(ctrl)
	c := mocks.NewCaptchaProviderMock(ctrl)
	h := NewHttp(config.Config{}, s, c, &mockCSRFSeeder{})

	s.EXPECT().Healthcheck().Return(nil)

	err := h.Healthcheck()

	assert.NoError(t, err)
}
