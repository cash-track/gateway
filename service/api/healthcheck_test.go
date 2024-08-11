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
	"github.com/cash-track/gateway/mocks"
)

const endpoint = "http://api.test.com"

func TestHealthcheckOk(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusOK)

		assert.NotNil(t, req)
		assert.Equal(t, fasthttp.MethodGet, string(req.Header.Method()))
		assert.Equal(t, fmt.Sprintf("%s%s", endpoint, healthcheckURI), req.URI().String())
		assert.Equal(t, string(headers.ContentTypeJson), string(req.Header.ContentType()))
		assert.Equal(t, string(headers.ContentTypeJson), string(req.Header.Peek(headers.Accept)))

		return nil
	})

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})
	err := s.Healthcheck()

	assert.NoError(t, err)
}

func TestHealthcheckFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(req *fasthttp.Request, resp *fasthttp.Response) error {
		resp.SetStatusCode(fasthttp.StatusInternalServerError)
		assert.NotNil(t, req)
		return nil
	})

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})
	err := s.Healthcheck()

	assert.Error(t, err)
}

func TestHealthcheckError(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))
	h.EXPECT().Do(gomock.Any(), gomock.Any()).Return(fmt.Errorf("connection reset by peer"))

	apiUrl, _ := url.Parse(endpoint)
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})
	err := s.Healthcheck()

	assert.Error(t, err)
}
