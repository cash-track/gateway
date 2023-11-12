package client

import (
	"strings"
	"time"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
)

func init() {
	NewClient()
}

const Service = "API"

var (
	client          *fasthttp.Client
	methodsWithBody = map[string]bool{
		fasthttp.MethodPost:  true,
		fasthttp.MethodPut:   true,
		fasthttp.MethodPatch: true,
	}
)

func NewClient() {
	client = &fasthttp.Client{
		ReadTimeout:                   500 * time.Millisecond,
		WriteTimeout:                  time.Second,
		MaxIdleConnDuration:           time.Hour,
		NoDefaultUserAgentHeader:      true,
		DisableHeaderNamesNormalizing: true,
		DisablePathNormalizing:        true,
		Dial: (&fasthttp.TCPDialer{
			Concurrency:      4096,
			DNSCacheDuration: time.Hour,
		}).Dial,
	}
}

func prepareRequestURI(sourcePath []byte, sourceQuery []byte) string {
	path := strings.TrimPrefix(string(sourcePath), "/api")

	if sourceQuery != nil {
		return config.Global.ApiUrl + path + "?" + string(sourceQuery)
	}

	return config.Global.ApiUrl + path
}
