package traces

import (
	"context"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestFindParentContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want context.Context
	}{
		{
			name: "nil context returns background",
			ctx:  nil,
			want: context.Background(),
		},
		{
			name: "context without trace returns original",
			ctx:  context.Background(),
			want: context.Background(),
		},
		{
			name: "context with trace returns stored context",
			ctx:  context.WithValue(context.Background(), traceCtxKey, context.TODO()),
			want: context.TODO(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindParentContext(tt.ctx)
			if got != tt.want {
				t.Errorf("FindParentContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateParentContext(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	spanCtx := context.TODO()

	UpdateParentContext(ctx, spanCtx)

	if got := ctx.UserValue(traceCtxKey); got != spanCtx {
		t.Errorf("UpdateParentContext() stored %v, want %v", got, spanCtx)
	}
}

func TestFindTraceId(t *testing.T) {
	tests := []struct {
		name     string
		setupCtx func() *fasthttp.RequestCtx
		want     string
	}{
		{
			name: "context with trace ID returns ID",
			setupCtx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.SetUserValue(traceIdCtxKey, "test-trace-id")
				return ctx
			},
			want: "test-trace-id",
		},
		{
			name: "context without trace ID returns empty string",
			setupCtx: func() *fasthttp.RequestCtx {
				return &fasthttp.RequestCtx{}
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			if got := FindTraceId(ctx); got != tt.want {
				t.Errorf("FindTraceId() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTraceHandler(t *testing.T) {
	for name, test := range map[string]struct {
		statusCode int
	}{
		"OK": {
			statusCode: fasthttp.StatusOK,
		},
		"BadRequest": {
			statusCode: fasthttp.StatusBadRequest,
		},
	} {
		t.Run(name, func(t *testing.T) {
			called := false
			handler := TraceHandler(func(ctx *fasthttp.RequestCtx) {
				called = true

				// Verify trace context is set
				if FindTraceId(ctx) == "" {
					t.Error("TraceHandler did not set trace ID")
				}

				// Test different response codes
				ctx.SetStatusCode(test.statusCode)
			})

			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod("GET")
			ctx.Request.SetRequestURI("/test")

			handler(ctx)

			if !called {
				t.Error("Handler was not called")
			}
		})
	}
}
