package retryhttp

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	httpmock "github.com/cash-track/gateway/mocks/http"
)

func TestRetriesTotalIncrementsOnRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := httpmock.NewClientMock(ctrl)
	c.EXPECT().Do(gomock.Any(), gomock.Any()).Times(2).Return(fmt.Errorf("unknown error: broken pipe"))

	client := FastHttpRetryClient{Client: c}
	client.WithRetryAttempts(2)

	before := testutil.ToFloat64(retriesTotal)
	err := client.Do(&fasthttp.Request{}, &fasthttp.Response{})

	assert.Error(t, err)
	assert.Equal(t, before+1, testutil.ToFloat64(retriesTotal))
}

func TestRetriesTotalUnchangedWithoutRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := httpmock.NewClientMock(ctrl)
	c.EXPECT().Do(gomock.Any(), gomock.Any()).Times(1).Return(nil)

	client := FastHttpRetryClient{Client: c}
	client.WithRetryAttempts(2)

	before := testutil.ToFloat64(retriesTotal)
	assert.NoError(t, client.Do(&fasthttp.Request{}, &fasthttp.Response{}))
	assert.Equal(t, before, testutil.ToFloat64(retriesTotal))
}
