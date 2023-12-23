package client

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/headers/cookie"
)

func TestRefreshTokenOk(t *testing.T) {
	const (
		oldAccessToken  = "test_old_access_token"
		oldRefreshToken = "test_old_refresh_token"
		newAccessToken  = "test_new_access_token"
		newRefreshToken = "test_new_refresh_token"
	)

	config.Global.ApiURI, _ = url.Parse(endpoint)

	mock := &MockClient{}
	mock.MockResponse(func(resp *fasthttp.Response) {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString(fmt.Sprintf(`{"accessToken":"%s","refreshToken":"%s"}`, newAccessToken, newRefreshToken))
	})

	auth := cookie.Auth{
		RefreshToken: oldRefreshToken,
		AccessToken:  oldAccessToken,
	}

	client = mock
	newAuth, err := refreshToken(auth)

	assert.NoError(t, err)
	assert.NotNil(t, mock.req)
	assert.Equal(t, fasthttp.MethodPost, string(mock.req.Header.Method()))
	assert.Equal(t, fmt.Sprintf("%s%s", endpoint, refreshURI), mock.req.URI().String())
	assert.Equal(t, string(headers.ContentTypeJson), string(mock.req.Header.ContentType()))
	assert.Equal(t, string(headers.ContentTypeJson), string(mock.req.Header.Peek(headers.Accept)))
	assert.Equal(t, fmt.Sprintf("Bearer %s", oldRefreshToken), string(mock.req.Header.Peek(headers.Authorization)))
	assert.Equal(t, fmt.Sprintf(`{"accessToken":"%s"}`, oldAccessToken), string(mock.req.Body()))

	assert.NotEmpty(t, newAuth)
	assert.Equal(t, newAccessToken, newAuth.AccessToken)
	assert.Equal(t, newRefreshToken, newAuth.RefreshToken)
}

func TestRefreshTokenFail(t *testing.T) {
	config.Global.ApiURI, _ = url.Parse(endpoint)

	mock := &MockClient{}
	mock.MockResponse(func(resp *fasthttp.Response) {
		resp.SetStatusCode(fasthttp.StatusInternalServerError)
		resp.SetBodyString(`{"error":"user deleted"}`)
	})

	auth := cookie.Auth{}

	client = mock
	newAuth, err := refreshToken(auth)

	assert.Error(t, err)
	assert.Empty(t, newAuth.AccessToken)
	assert.Empty(t, newAuth.RefreshToken)
}

func TestRefreshTokenError(t *testing.T) {
	config.Global.ApiURI, _ = url.Parse(endpoint)

	mock := &MockClient{}
	mock.MockResponse(func(resp *fasthttp.Response) {
		resp.SetStatusCode(fasthttp.StatusOK)
	})
	mock.ReturnError(fmt.Errorf("context cancelled"))

	auth := cookie.Auth{}

	client = mock
	newAuth, err := refreshToken(auth)

	assert.Error(t, err)
	assert.Empty(t, newAuth.AccessToken)
	assert.Empty(t, newAuth.RefreshToken)
}

func TestRefreshTokenErrorBadResponse(t *testing.T) {
	config.Global.ApiURI, _ = url.Parse(endpoint)

	mock := &MockClient{}
	mock.MockResponse(func(resp *fasthttp.Response) {
		resp.SetStatusCode(fasthttp.StatusOK)
		resp.SetBodyString("{")
	})

	auth := cookie.Auth{}

	client = mock
	newAuth, err := refreshToken(auth)

	assert.Error(t, err)
	assert.Empty(t, newAuth.AccessToken)
	assert.Empty(t, newAuth.RefreshToken)
}

func TestRefreshTokenErrorLoggedOff(t *testing.T) {
	config.Global.ApiURI, _ = url.Parse(endpoint)

	mock := &MockClient{}
	mock.MockResponse(func(resp *fasthttp.Response) {
		resp.SetStatusCode(fasthttp.StatusUnauthorized)
		resp.SetBodyString(`{"message":"refresh token expired"}`)
	})

	auth := cookie.Auth{}

	client = mock
	newAuth, err := refreshToken(auth)

	assert.NoError(t, err)
	assert.Empty(t, newAuth.AccessToken)
	assert.Empty(t, newAuth.RefreshToken)
}
