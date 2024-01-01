package router

import (
	"testing"

	"github.com/cash-track/gateway/config"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	r := New(config.Config{})

	l := r.List()

	assert.Len(t, l, 2)

	assert.NotNil(t, l["*"])
	assert.Len(t, l["*"], 3)
	assert.Contains(t, l["*"], "/live")
	assert.Contains(t, l["*"], "/ready")
	assert.Contains(t, l["*"], "/api/{path:*}")

	assert.NotNil(t, l["POST"])
	assert.Len(t, l["POST"], 4)
	assert.Contains(t, l["POST"], "/api/auth/login")
	assert.Contains(t, l["POST"], "/api/auth/logout")
	assert.Contains(t, l["POST"], "/api/auth/register")
	assert.Contains(t, l["POST"], "/api/auth/provider/google")
}
