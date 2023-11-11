package client

import (
	"encoding/json"
	"fmt"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/logger"
)

var refreshURI = []byte("/auth/refresh")

func refreshToken(auth cookie.Auth) (cookie.Auth, error) {
	req := fasthttp.AcquireRequest()
	defer func() {
		fasthttp.ReleaseRequest(req)
	}()

	req.Header.SetMethod(fasthttp.MethodPost)
	req.SetRequestURI(prepareRequestURI(refreshURI, nil))
	req.Header.SetContentTypeBytes(headers.ContentTypeJson)
	req.Header.SetBytesV(headers.Accept, headers.ContentTypeJson)
	headers.WriteBearerToken(req, auth.RefreshToken)

	data, _ := json.Marshal(cookie.Auth{AccessToken: auth.AccessToken})
	req.SetBody(data)

	logger.DebugRequest(req, Service)

	// execute request
	resp := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseResponse(resp)
	}()

	newAuth := cookie.Auth{}
	err := client.Do(req, resp)
	if err != nil {
		return newAuth, fmt.Errorf("refresh token API request error: %w", err)
	}

	logger.DebugResponse(resp, Service)

	if resp.StatusCode() != fasthttp.StatusOK {
		return newAuth, fmt.Errorf("refresh token failed")
	}

	if err := json.Unmarshal(resp.Body(), &newAuth); err != nil {
		return newAuth, fmt.Errorf("refresh token unexpected response body: %w", err)
	}

	if !newAuth.IsLogged() {
		return newAuth, fmt.Errorf("refresh token unsuccessful")
	}

	return newAuth, nil
}
