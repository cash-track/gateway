package headers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestGetClientIPFromContext(t *testing.T) {
	ctx := fasthttp.RequestCtx{}

	clientIp := GetClientIPFromContext(&ctx)
	assert.Equal(t, "0.0.0.0", clientIp)

	ctx.SetUserValueBytes(clientIpUserValue, "192.168.1.3")

	clientIp = GetClientIPFromContext(&ctx)
	assert.Equal(t, "192.168.1.3", clientIp)
}

func TestFindRealClientIP(t *testing.T) {

	for name, test := range map[string]struct {
		Headers    map[string]string
		ExpectedIP string
	}{
		"CloudFlare": {
			ExpectedIP: "192.168.1.2",
			Headers: map[string]string{
				CfConnectingIP: "192.168.1.2",
				XRealIp:        "192.168.1.1",
				XForwardedFor:  "192.168.1.0",
			},
		},
		"Real": {
			ExpectedIP: "192.168.1.1",
			Headers: map[string]string{
				CfConnectingIP: "",
				XRealIp:        "192.168.1.1",
				XForwardedFor:  "192.168.1.0",
			},
		},
		"Forwarded": {
			ExpectedIP: "192.168.1.0",
			Headers: map[string]string{
				CfConnectingIP: "",
				XForwardedFor:  "192.168.1.0",
			},
		},
		"RemoteAddress": {
			ExpectedIP: "0.0.0.0",
			Headers:    map[string]string{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := fasthttp.RequestCtx{}

			for key, value := range test.Headers {
				ctx.Request.Header.Set(key, value)
			}

			clientIp := findRealClientIP(&ctx)
			assert.Equal(t, test.ExpectedIP, clientIp)
		})
	}

}
