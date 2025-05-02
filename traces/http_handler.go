package traces

import (
	"context"
	"fmt"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func FindParentContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	if c, ok := ctx.Value(traceCtxKey).(context.Context); ok {
		return c
	}

	return ctx
}

func UpdateParentContext(ctx *fasthttp.RequestCtx, spanCtx context.Context) {
	ctx.SetUserValue(traceCtxKey, spanCtx)
}

func FindTraceId(ctx *fasthttp.RequestCtx) string {
	if id, ok := ctx.Value(traceIdCtxKey).(string); ok {
		return id
	}

	return ""
}

func TraceHandler(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	tracer := GetTracer()

	return func(ctx *fasthttp.RequestCtx) {
		spanCtx, span := tracer.Start(
			context.Background(),
			fmt.Sprintf("%s %s", ctx.Request.Header.Method(), ctx.Path()),
			trace.WithAttributes(
				RequestAttributes(&ctx.Request)...,
			),
		)
		defer span.End()

		// Store trace context in RequestCtx for downstream handlers
		UpdateParentContext(ctx, spanCtx)
		ctx.SetUserValue(traceIdCtxKey, span.SpanContext().TraceID().String())

		next(ctx)

		span.SetAttributes(ResponseAttributes(&ctx.Response)...)

		if ctx.Response.StatusCode() < fasthttp.StatusBadRequest {
			span.SetStatus(codes.Ok, "")
		} else {
			span.SetStatus(codes.Error, "")
		}
	}
}
