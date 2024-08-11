package api

import (
	"fmt"
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

func TestRefreshTokenOk(t *testing.T) {
	const (
		oldAccessToken  = "test_old_access_token"
		oldRefreshToken = "test_old_refresh_token"
		newAccessToken  = "test_new_access_token"
		newRefreshToken = "test_new_refresh_token"
	)

	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(fmt.Sprintf(`{"accessToken":"%s","refreshToken":"%s"}`, newAccessToken, newRefreshToken))

		assert.NotNil(t, req)
		assert.Equal(t, fasthttp.MethodPost, string(req.Header.Method()))
		assert.Equal(t, fmt.Sprintf("%s%s", endpoint, refreshURI), req.URI().String())
		assert.Equal(t, string(headers.ContentTypeJson), string(req.Header.ContentType()))
		assert.Equal(t, string(headers.ContentTypeJson), string(req.Header.Peek(headers.Accept)))
		assert.Equal(t, fmt.Sprintf("Bearer %s", oldRefreshToken), string(req.Header.Peek(headers.Authorization)))
		assert.Equal(t, fmt.Sprintf(`{"accessToken":"%s"}`, oldAccessToken), string(req.Body()))

		return nil
	})

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	auth := cookie.Auth{
		RefreshToken: oldRefreshToken,
		AccessToken:  oldAccessToken,
	}

	newAuth, err := s.refreshToken(auth)

	assert.NoError(t, err)
	assert.NotEmpty(t, newAuth)
	assert.Equal(t, newAccessToken, newAuth.AccessToken)
	assert.Equal(t, newRefreshToken, newAuth.RefreshToken)
}

func TestRefreshTokenFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusInternalServerError)
		resp.SetBodyString(`{"error":"user deleted"}`)
		assert.NotNil(t, req)
		return nil
	})

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	auth := cookie.Auth{}

	newAuth, err := s.refreshToken(auth)

	assert.Error(t, err)
	assert.Empty(t, newAuth.AccessToken)
	assert.Empty(t, newAuth.RefreshToken)
}

func TestRefreshTokenError(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(fmt.Errorf("context cancelled"))

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	auth := cookie.Auth{}

	newAuth, err := s.refreshToken(auth)

	assert.Error(t, err)
	assert.Empty(t, newAuth.AccessToken)
	assert.Empty(t, newAuth.RefreshToken)
}

func TestRefreshTokenErrorBadResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString("{")
		assert.NotNil(t, req)
		return nil
	})

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	auth := cookie.Auth{}

	newAuth, err := s.refreshToken(auth)

	assert.Error(t, err)
	assert.Empty(t, newAuth.AccessToken)
	assert.Empty(t, newAuth.RefreshToken)
}

func TestRefreshTokenErrorLoggedOff(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusUnauthorized)
		resp.SetBodyString(`{"message":"refresh token expired"}`)
		assert.NotNil(t, req)
		return nil
	})

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	auth := cookie.Auth{}

	newAuth, err := s.refreshToken(auth)

	assert.NoError(t, err)
	assert.Empty(t, newAuth.AccessToken)
	assert.Empty(t, newAuth.RefreshToken)
}
