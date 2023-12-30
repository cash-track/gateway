package client

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/headers"
)

const endpoint = "http://api.test.com"

func TestHealthcheckOk(t *testing.T) {
	config.Global.ApiURI, _ = url.Parse(endpoint)

	mock := &MockClient{}
	mock.MockResponse(func(resp *fasthttp.Response) {
		resp.SetStatusCode(fasthttp.StatusOK)
	})

	client = mock
	err := Healthcheck()

	assert.NoError(t, err)
	assert.NotNil(t, mock.GetRequest())
	assert.Equal(t, fasthttp.MethodGet, string(mock.GetRequest().Header.Method()))
	assert.Equal(t, fmt.Sprintf("%s%s", endpoint, healthcheckURI), mock.GetRequest().URI().String())
	assert.Equal(t, string(headers.ContentTypeJson), string(mock.GetRequest().Header.ContentType()))
	assert.Equal(t, string(headers.ContentTypeJson), string(mock.GetRequest().Header.Peek(headers.Accept)))
}

func TestHealthcheckFail(t *testing.T) {
	config.Global.ApiURI, _ = url.Parse(endpoint)

	mock := &MockClient{}
	mock.MockResponse(func(resp *fasthttp.Response) {
		resp.SetStatusCode(fasthttp.StatusInternalServerError)
	})

	client = mock
	err := Healthcheck()

	assert.Error(t, err)
	assert.NotNil(t, mock.GetRequest())
}

func TestHealthcheckError(t *testing.T) {
	config.Global.ApiURI, _ = url.Parse(endpoint)

	mock := &MockClient{}
	mock.ReturnError(fmt.Errorf("connection reset by peer"))
	mock.MockResponse(func(resp *fasthttp.Response) {
		resp.SetStatusCode(fasthttp.StatusOK)
	})

	client = mock
	err := Healthcheck()

	assert.Error(t, err)
	assert.NotNil(t, mock.GetRequest())
}
