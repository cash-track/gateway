package traces

import (
	"context"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func PropagateContextToRequest(ctx context.Context, req *fasthttp.Request) {
	otel.GetTextMapPropagator().Inject(
		FindParentContext(ctx),
		newFastHttpCarrier(&req.Header),
	)
}

// fastHttpCarrier implements TextMapCarrier for fasthttp.RequestHeader
type fastHttpCarrier struct {
	header *fasthttp.RequestHeader
}

func newFastHttpCarrier(header *fasthttp.RequestHeader) propagation.TextMapCarrier {
	return &fastHttpCarrier{header: header}
}

func (c *fastHttpCarrier) Get(key string) string {
	return string(c.header.Peek(key))
}

func (c *fastHttpCarrier) Set(key, value string) {
	c.header.Set(key, value)
}

func (c *fastHttpCarrier) Keys() []string {
	var keys []string
	c.header.VisitAll(func(key, _ []byte) {
		keys = append(keys, string(key))
	})
	return keys
}
