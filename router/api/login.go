package api

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/traces"
)

func (h *HttpHandler) Login(ctx *fasthttp.RequestCtx) error {
	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		return nil
	}

	auth := cookie.Auth{}
	if err := json.Unmarshal(ctx.Response.Body(), &auth); err != nil {
		return fmt.Errorf("login response body invalid: %w", err)
	}

	if err := auth.WriteCookie(ctx); err != nil {
		return fmt.Errorf("login write cookie: %w", err)
	}

	// Seed the initial CSRF token. Non-fatal: if Redis is unavailable the user
	// will recover automatically on their first mutation via GET /csrf.
	if err := h.csrf.Seed(ctx, auth); err != nil {
		slog.Warn("csrf seed failed after login", "trace_id", traces.FindTraceId(ctx), "error", err)
	}

	b, _ := h.newWebAppRedirect().ToJson()
	ctx.Response.SetBody(b)

	return nil
}
