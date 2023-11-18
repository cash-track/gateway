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
		ReadTimeout:                   5 * time.Second,
		WriteTimeout:                  5 * time.Second,
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

func setRequestURI(dest *fasthttp.URI, path []byte) {
	_ = dest.Parse([]byte(config.Global.ApiUrl), nil)
	dest.SetScheme(config.Global.ApiURI.Scheme)
	dest.SetHost(config.Global.ApiURI.Host)
	dest.SetPathBytes(path)
}

func copyRequestURI(src, dest *fasthttp.URI) {
	path := strings.TrimPrefix(string(src.PathOriginal()), "/api")
	setRequestURI(dest, []byte(path))
	dest.SetQueryStringBytes(src.QueryString())
}
