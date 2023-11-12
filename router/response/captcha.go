package response

import "github.com/valyala/fasthttp"

func NewCaptchaBadResponse() ErrorResponse {
	return NewErrorResponse("Captcha validation unsuccessful. Please try again.", nil, fasthttp.StatusBadRequest)
}

func NewCaptchaErrorResponse(err error) ErrorResponse {
	return NewErrorResponse(
		"Unexpected response from captcha validation service. Please try again later.",
		err,
		fasthttp.StatusInternalServerError,
	)
}
