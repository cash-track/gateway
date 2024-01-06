package api

import (
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/mocks"
)

func TestFullForwardRequestWithAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)

		assert.NotNil(t, req)
		assert.Equal(t, fasthttp.MethodPatch, string(req.Header.Method()))
		assert.Equal(t, fmt.Sprintf("%s%s", endpoint, "/auth/profile"), req.URI().String())
		assert.Equal(t, string(headers.ContentTypeJson), string(req.Header.ContentType()))
		assert.Equal(t, string(headers.ContentTypeJson), string(req.Header.Peek(headers.Accept)))
		assert.Equal(t, "10.0.0.1", string(req.Header.Peek(headers.XForwardedFor)))
		assert.Equal(t, "Bearer access_token", string(req.Header.Peek(headers.Authorization)))
		assert.Equal(t, `{"status":"ok"}`, string(req.Body()))

		return nil
	})

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	uri := &fasthttp.URI{}
	_ = uri.Parse(nil, []byte("https://gateway.test.com/api/auth/profile"))

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPatch)
	ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, "access_token")
	ctx.Request.SetURI(uri)
	ctx.Request.SetBodyString(`{"status":"ok"}`)
	ctx.SetRemoteAddr(&net.TCPAddr{IP: []byte{0xA, 0x0, 0x0, 0x1}})

	err := s.ForwardRequest(&ctx, nil)

	assert.NoError(t, err)
}

func TestForwardRequestWithBodyOverride(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)

		assert.NotNil(t, req)
		assert.Equal(t, fasthttp.MethodPost, string(req.Header.Method()))
		assert.Equal(t, `{"status":"false"}`, string(req.Body()))

		return nil
	})

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetBodyString(`{"status":"ok"}`)

	err := s.ForwardRequest(&ctx, []byte(`{"status":"false"}`))

	assert.NoError(t, err)
}

func TestForwardRequestWithAuthRefresh(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusUnauthorized)

		assert.NotNil(t, req)
		assert.Equal(t, fasthttp.MethodGet, string(req.Header.Method()))
		assert.Equal(t, "Bearer access_token", string(req.Header.Peek(headers.Authorization)))

		return nil
	})
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(fmt.Sprintf(`{"accessToken":"%s","refreshToken":"%s"}`, "new_access_token", "new_refresh_token"))
		return nil
	})
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)

		assert.NotNil(t, req)
		assert.Equal(t, fasthttp.MethodGet, string(req.Header.Method()))
		assert.Equal(t, "Bearer new_access_token", string(req.Header.Peek(headers.Authorization)))

		return nil
	})

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, "access_token")
	ctx.Request.Header.SetCookie(cookie.RefreshTokenCookieName, "refresh_token")

	err := s.ForwardRequest(&ctx, nil)

	assert.NoError(t, err)
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), "new_access_token")
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), "new_refresh_token")
}

func TestForwardRequestWithAuthRefreshFailLogout(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusUnauthorized)

		assert.NotNil(t, req)
		assert.Equal(t, fasthttp.MethodGet, string(req.Header.Method()))
		assert.Equal(t, "Bearer access_token", string(req.Header.Peek(headers.Authorization)))

		return nil
	})
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(fmt.Errorf("broken pipe"))

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, "access_token")
	ctx.Request.Header.SetCookie(cookie.RefreshTokenCookieName, "refresh_token")

	err := s.ForwardRequest(&ctx, nil)

	assert.NoError(t, err)

	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.AccessTokenCookieName)), fmt.Sprintf("%s=;", cookie.AccessTokenCookieName))
	assert.Contains(t, string(ctx.Response.Header.PeekCookie(cookie.RefreshTokenCookieName)), fmt.Sprintf("%s=;", cookie.RefreshTokenCookieName))
}

func TestForwardRequestWithAuthRefreshSecondFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusUnauthorized)

		assert.NotNil(t, req)
		assert.Equal(t, fasthttp.MethodGet, string(req.Header.Method()))
		assert.Equal(t, "Bearer access_token", string(req.Header.Peek(headers.Authorization)))

		return nil
	})
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(fmt.Sprintf(`{"accessToken":"%s","refreshToken":"%s"}`, "new_access_token", "new_refresh_token"))

		return nil
	})
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		assert.NotNil(t, req)
		assert.Equal(t, fasthttp.MethodGet, string(req.Header.Method()))
		assert.Equal(t, "Bearer new_access_token", string(req.Header.Peek(headers.Authorization)))

		return fmt.Errorf("broken pipe")
	})

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, "access_token")
	ctx.Request.Header.SetCookie(cookie.RefreshTokenCookieName, "refresh_token")

	err := s.ForwardRequest(&ctx, nil)

	assert.Error(t, err)
}

func TestForwardRequestError(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(fmt.Errorf("broken pipe"))

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)

	err := s.ForwardRequest(&ctx, nil)

	assert.Error(t, err)
}

func TestForwardResponse(t *testing.T) {
	ctx := fasthttp.RequestCtx{}

	resp := fasthttp.Response{}
	resp.SetStatusCode(fasthttp.StatusUnauthorized)
	resp.SetBody([]byte("not allowed\n\r"))
	resp.Header.Set(headers.AccessControlAllowOrigin, "test.com")
	resp.Header.Set(headers.AccessControlMaxAge, "3600")
	resp.Header.Set(headers.AccessControlAllowMethods, "GET,POST")
	resp.Header.Set(headers.AccessControlAllowHeaders, "Content-Type,Accept-Language")
	resp.Header.Set(headers.ContentType, "text/plain")
	resp.Header.Set(headers.RetryAfter, "200")
	resp.Header.Set(headers.Vary, "Content-Type,X-Rate-Limit")
	resp.Header.Set(headers.XRateLimit, "123")
	resp.Header.Set(headers.XRateLimitRemaining, "2")

	err := forwardResponse(&ctx, &resp)

	assert.NoError(t, err)

	assert.Equal(t, "test.com", string(ctx.Response.Header.Peek(headers.AccessControlAllowOrigin)))
	assert.Equal(t, "true", string(ctx.Response.Header.Peek(headers.AccessControlAllowCredentials)))
	assert.Equal(t, "GET,POST", string(ctx.Response.Header.Peek(headers.AccessControlAllowMethods)))
	assert.Equal(t, "Content-Type,Accept-Language", string(ctx.Response.Header.Peek(headers.AccessControlAllowHeaders)))
	assert.Equal(t, "3600", string(ctx.Response.Header.Peek(headers.AccessControlMaxAge)))
	assert.Equal(t, "text/plain", string(ctx.Response.Header.Peek(headers.ContentType)))
	assert.Equal(t, "200", string(ctx.Response.Header.Peek(headers.RetryAfter)))
	assert.Equal(t, "Content-Type,X-Rate-Limit", string(ctx.Response.Header.Peek(headers.Vary)))
	assert.Equal(t, "123", string(ctx.Response.Header.Peek(headers.XRateLimit)))
	assert.Equal(t, "2", string(ctx.Response.Header.Peek(headers.XRateLimitRemaining)))
}
