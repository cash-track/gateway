package response

import (
	"encoding/json"
	"errors"

	"github.com/valyala/fasthttp"
)

const DefaultErrorMessage = "Unexpected error happened. Please try again later."

type ErrorResponse struct {
	Message    string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
	StatusCode int    `json:"-"`
}

func (e ErrorResponse) Write(ctx *fasthttp.RequestCtx) {
	body, _ := json.Marshal(e)
	ctx.Response.SetBody(body)
	ctx.Response.SetStatusCode(e.StatusCode)
	ctx.Response.Header.Set("Content-Type", "application/json")
}

func NewErrorResponse(message string, err error, status int) ErrorResponse {
	if err == nil {
		err = errors.New("")
	}

	return ErrorResponse{
		Message:    message,
		Error:      err.Error(),
		StatusCode: status,
	}
}

func ByError(err error) ErrorResponse {
	return NewErrorResponse(DefaultErrorMessage, err, fasthttp.StatusInternalServerError)
}

func ByErrorAndStatus(err error, status int) ErrorResponse {
	return NewErrorResponse(DefaultErrorMessage, err, status)
}
