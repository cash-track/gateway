package api

import (
	"fmt"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/headers"
	"github.com/cash-track/gateway/logger"
)

var healthcheckURI = []byte("/healthcheck")

func (s *HttpService) Healthcheck() error {
	req := fasthttp.AcquireRequest()
	defer func() {
		fasthttp.ReleaseRequest(req)
	}()

	req.Header.SetMethod(fasthttp.MethodGet)
	s.setRequestURI(req.URI(), healthcheckURI)
	req.Header.SetContentTypeBytes(headers.ContentTypeJson)
	req.Header.SetBytesV(headers.Accept, headers.ContentTypeJson)

	logger.DebugRequest(req, ServiceId)

	// execute request
	resp := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseResponse(resp)
	}()

	err := s.http.Do(req, resp)
	if err != nil {
		return fmt.Errorf("healthckeck API request error: %w", err)
	}

	logger.DebugResponse(resp, ServiceId)

	if resp.StatusCode() != fasthttp.StatusOK {
		return fmt.Errorf("healthckeck failed [%d], body: %s", resp.StatusCode(), resp.Body())
	}

	return nil
}
