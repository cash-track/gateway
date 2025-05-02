package traces

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func TestPropagateContextToRequest(t *testing.T) {
	// Setup
	req := &fasthttp.Request{}
	req.Header.Set("Existing-Header", "value")

	// Create a context with trace information
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	otel.SetTextMapPropagator(prop)

	ctx := context.Background()
	PropagateContextToRequest(ctx, req)

	// Test that the propagation headers were set
	carrier := newFastHttpCarrier(&req.Header)
	assert.NotEmpty(t, carrier.Keys())
	assert.Equal(t, "value", carrier.Get("Existing-Header"))
}

func TestFastHttpCarrier(t *testing.T) {
	for name, test := range map[string]struct {
		get    func() *fasthttp.RequestHeader
		verify func(t *testing.T, r *fasthttp.RequestHeader, c propagation.TextMapCarrier)
	}{
		"Get": {
			get: func() *fasthttp.RequestHeader {
				header := &fasthttp.RequestHeader{}
				header.Set("Test-Key", "test-value")
				return header
			},
			verify: func(t *testing.T, r *fasthttp.RequestHeader, c propagation.TextMapCarrier) {
				value := c.Get("Test-Key")
				assert.Equal(t, "test-value", value)
				assert.Empty(t, c.Get("Non-Existent"))
			},
		},
		"Set": {
			get: func() *fasthttp.RequestHeader {
				return &fasthttp.RequestHeader{}
			},
			verify: func(t *testing.T, r *fasthttp.RequestHeader, c propagation.TextMapCarrier) {
				c.Set("New-Key", "new-value")
				assert.Equal(t, "new-value", string(r.Peek("New-Key")))
			},
		},
		"Keys": {
			get: func() *fasthttp.RequestHeader {
				header := &fasthttp.RequestHeader{}
				header.Set("Key1", "value1")
				header.Set("Key2", "value2")
				return header
			},
			verify: func(t *testing.T, r *fasthttp.RequestHeader, c propagation.TextMapCarrier) {
				keys := c.Keys()
				assert.Contains(t, keys, "Key1")
				assert.Contains(t, keys, "Key2")
				assert.Len(t, keys, 2)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			header := test.get()
			carrier := newFastHttpCarrier(header)
			test.verify(t, header, carrier)
		})
	}
}
