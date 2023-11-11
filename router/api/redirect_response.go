package api

import (
	"encoding/json"

	"github.com/cash-track/gateway/config"
)

type redirectResponse struct {
	RedirectUrl string `json:"redirectUrl"`
}

func newWebAppRedirect() *redirectResponse {
	return newRedirectResponse(config.Global.WebAppUrl)
}

func newWebsiteRedirect() *redirectResponse {
	return newRedirectResponse(config.Global.WebsiteUrl)
}

func newRedirectResponse(url string) *redirectResponse {
	return &redirectResponse{RedirectUrl: url}
}

func (r *redirectResponse) ToJson() ([]byte, error) {
	return json.Marshal(r)
}
