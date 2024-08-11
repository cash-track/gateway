package api

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/config"
	"github.com/cash-track/gateway/mocks"
)

func TestNewClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))

	s := NewHttp(h, config.Config{})

	assert.NotNil(t, s.http)
}

func TestSetRequestURI(t *testing.T) {
	apiUrl, _ := url.Parse("http://api.test.com")

	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	uri := fasthttp.URI{}

	s.setRequestURI(&uri, []byte("/users/create one"))

	assert.Equal(t, "http://api.test.com/users/create%20one", uri.String())
}

func TestCopyRequestURI(t *testing.T) {
	apiUrl, _ := url.Parse("http://api.test.com")

	ctrl := gomock.NewController(t)
	h := mocks.NewHttpRetryClientMock(ctrl)
	h.EXPECT().WithReadTimeout(gomock.Eq(httpReadTimeout))
	h.EXPECT().WithWriteTimeout(gomock.Eq(httpWriteTimeout))
	h.EXPECT().WithRetryAttempts(gomock.Eq(httpRetryAttempts))
	s := NewHttp(h, config.Config{
		ApiURI: apiUrl,
	})

	src := fasthttp.URI{}
	src.SetPath("/api/users/create one")
	src.SetQueryString("one=two%203")
	dest := fasthttp.URI{}

	s.copyRequestURI(&src, &dest)

	assert.Equal(t, "http://api.test.com/users/create%20one?one=two%203", dest.String())
}
