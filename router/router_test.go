package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.Len(t, l["GET"], 1)
	assert.Contains(t, l["GET"], "/csrf")

	assert.NotNil(t, l["POST"])
	assert.Len(t, l["POST"], 6)
	assert.Contains(t, l["POST"], "/api/auth/login")
	assert.Contains(t, l["POST"], "/api/auth/login/passkey")
	assert.Contains(t, l["POST"], "/api/auth/login/passkey/init")
	assert.Contains(t, l["POST"], "/api/auth/logout")
	assert.Contains(t, l["POST"], "/api/auth/register")
	assert.Contains(t, l["POST"], "/api/auth/provider/google")
}
