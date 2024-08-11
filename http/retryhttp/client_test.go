package retryhttp

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/http"
	httpmock "github.com/cash-track/gateway/mocks/http"
)

func TestNewFastHttpRetryClient(t *testing.T) {
	client := NewFastHttpRetryClient()
	assert.NotNil(t, client)
}

func TestDoWithRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := httpmock.NewClientMock(ctrl)
	c.EXPECT().Do(gomock.Any(), gomock.Any()).Times(2).Return(fmt.Errorf("unknown error: broken pipe or closed connection"))

	client := FastHttpRetryClient{
		Client: c,
	}

	client.WithRetryAttempts(2)
	err := client.Do(&fasthttp.Request{}, &fasthttp.Response{})

	assert.Error(t, err)
}

func TestWithRetryAttempts(t *testing.T) {
	client := FastHttpRetryClient{
		Client: &http.FastHttpClient{},
	}
	client.WithRetryAttempts(3)

	assert.Equal(t, uint(3), client.attempts)
}
