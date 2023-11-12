package client

import (
	"fmt"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/logger"
)

var healthcheckURI = []byte("/healthcheck")

func Healthcheck() error {
	req := fasthttp.AcquireRequest()
	defer func() {
		fasthttp.ReleaseRequest(req)
	}()

	req.Header.SetMethod(fasthttp.MethodGet)
	req.SetRequestURI(prepareRequestURI(healthcheckURI, nil))
	req.Header.SetContentTypeBytes(headers.ContentTypeJson)
	req.Header.SetBytesV(headers.Accept, headers.ContentTypeJson)

	logger.DebugRequest(req, Service)

	// execute request
	resp := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseResponse(resp)
	}()

	err := client.Do(req, resp)
	if err != nil {
		return fmt.Errorf("healthckeck API request error: %w", err)
	}

	logger.DebugResponse(resp, Service)

	if resp.StatusCode() != fasthttp.StatusOK {
		return fmt.Errorf("healthckeck failed [%d], body: %s", resp.StatusCode(), resp.Body())
	}

	return nil
}
