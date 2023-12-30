package client

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
)

func TestNewClient(t *testing.T) {
	NewClient()

	assert.NotNil(t, client)

	if c, ok := client.(*fasthttp.Client); ok {
		assert.True(t, c.NoDefaultUserAgentHeader)
		assert.True(t, c.DisableHeaderNamesNormalizing)
		assert.True(t, c.DisablePathNormalizing)
	}
}

func TestSetRequestURI(t *testing.T) {
	config.Global.ApiURI, _ = url.Parse("http://api.test.com")

	uri := fasthttp.URI{}

	setRequestURI(&uri, []byte("/users/create one"))

	assert.Equal(t, "http://api.test.com/users/create%20one", uri.String())
}

func TestCopyRequestURI(t *testing.T) {
	config.Global.ApiURI, _ = url.Parse("http://api.test.com")

	src := fasthttp.URI{}
	src.SetPath("/api/users/create one")
	src.SetQueryString("one=two%203")
	dest := fasthttp.URI{}

	copyRequestURI(&src, &dest)

	assert.Equal(t, "http://api.test.com/users/create%20one?one=two%203", dest.String())
}
