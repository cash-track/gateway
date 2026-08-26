package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/cash-track/gateway/mocks"
)

func TestNew(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := mocks.NewApiHandlerMock(ctrl)
	c := mocks.NewCsrfHandlerMock(ctrl)
	r := New(a, c)

	l := r.List()

	assert.Len(t, l, 3)

	assert.NotNil(t, l["*"])
	assert.Len(t, l["*"], 3)
	assert.Contains(t, l["*"], "/live")
	assert.Contains(t, l["*"], "/ready")
	assert.Contains(t, l["*"], "/api/{path:*}")

	assert.NotNil(t, l["GET"])
	assert.Len(t, l["GET"], 2)
	assert.Contains(t, l["GET"], "/csrf")
	// Passkey login init is captcha-verified on GET, matching the backend and the client.
	assert.Contains(t, l["GET"], "/api/auth/login/passkey/init")

	assert.NotNil(t, l["POST"])
	assert.Len(t, l["POST"], 5)
	assert.Contains(t, l["POST"], "/api/auth/login")
	assert.Contains(t, l["POST"], "/api/auth/login/passkey")
	assert.NotContains(t, l["POST"], "/api/auth/login/passkey/init")
	assert.Contains(t, l["POST"], "/api/auth/logout")
	assert.Contains(t, l["POST"], "/api/auth/register")
	assert.Contains(t, l["POST"], "/api/auth/provider/google")
}

// The table above only proves which paths are registered. These dispatch a real request to
// prove the passkey-init GET reaches the captcha handler and not the catch-all proxy.
func TestPasskeyLoginInitGetIsCaptchaVerified(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := mocks.NewApiHandlerMock(ctrl)
	c := mocks.NewCsrfHandlerMock(ctrl)
	a.EXPECT().CaptchaVerifyHandler(gomock.Any()).Times(1)

	r := New(a, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	ctx.Request.SetRequestURI("/api/auth/login/passkey/init")

	r.Handler(&ctx)
}

func TestPasskeyLoginInitPostIsProxied(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := mocks.NewApiHandlerMock(ctrl)
	c := mocks.NewCsrfHandlerMock(ctrl)
	a.EXPECT().FullForwardedHandler(gomock.Any()).Times(1)

	r := New(a, c)

	ctx := fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetRequestURI("/api/auth/login/passkey/init")

	r.Handler(&ctx)
}
