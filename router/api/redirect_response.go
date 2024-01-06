package api

import (
	"encoding/json"
)

type redirectResponse struct {
	RedirectUrl string `json:"redirectUrl"`
}

func newRedirectResponse(url string) *redirectResponse {
	return &redirectResponse{RedirectUrl: url}
}

func (r *redirectResponse) ToJson() ([]byte, error) {
	return json.Marshal(r)
}

func (h *HttpHandler) newWebAppRedirect() *redirectResponse {
	return newRedirectResponse(h.config.WebAppUrl)
}

func (h *HttpHandler) newWebsiteRedirect() *redirectResponse {
	return newRedirectResponse(h.config.WebsiteUrl)
}
