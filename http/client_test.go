package http

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestNewFastHttpClient(t *testing.T) {
	client := NewFastHttpClient()
	assert.NotNil(t, client)
}

func TestWithReadTimeout(t *testing.T) {
	client := FastHttpClient{
		Client: &fasthttp.Client{},
	}
	client.WithReadTimeout(1 * time.Second)

	assert.Equal(t, 1*time.Second, client.ReadTimeout)
}

func TestWithWriteTimeout(t *testing.T) {
	client := FastHttpClient{
		Client: &fasthttp.Client{},
	}
	client.WithWriteTimeout(1 * time.Second)

	assert.Equal(t, 1*time.Second, client.WriteTimeout)
}
