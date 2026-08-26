package csrf

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/jwks"
	"github.com/cash-track/gateway/router/response"
	"github.com/cash-track/gateway/traces"
	"github.com/cash-track/gateway/traces/semconv"
)

const (
	keyPrefix         = "CT:csrf"
	tokenTtl          = time.Minute * 10
	metricsNamespace  = "gateway"
	metricsCsrfSubsys = "csrf"
)

var csrfRotationFailedTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsCsrfSubsys,
	Name:      "rotation_failed_total",
	Help:      "CSRF token rotations that failed after a successful response, leaving the response untouched.",
})

var csrfValidationFailedOpenTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsCsrfSubsys,
	Name:      "validation_failed_open_total",
	Help:      "CSRF validations that failed open because Redis was unreachable (not a missing/expired token).",
})

var csrfSignatureVerificationFailOpenTotal = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsCsrfSubsys,
	Name:      "signature_verification_fail_open_total",
	Help:      "Access tokens accepted without RS256 signature verification because no JWKS key material is loaded.",
})

// csrfRedisUp is updated as requests flow through, not by a dedicated health check.
var csrfRedisUp = newRedisUpGauge()

func newRedisUpGauge() prometheus.Gauge {
	g := promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsCsrfSubsys,
		Name:      "redis_up",
		Help:      "Whether the last CSRF Redis operation succeeded (1) or failed (0). Optimistic 1 before first use.",
	})
	g.Set(1)

	return g
}

// Endpoints that replace whatever session the request arrived with. Seed() owns the CSRF
// lifecycle for these; see Handler for why they bypass validation and rotation entirely.
var sessionEstablishingPaths = map[string]bool{
	"/api/auth/login":           true,
	"/api/auth/login/passkey":   true,
	"/api/auth/register":        true,
	"/api/auth/provider/google": true,
}

var (
	csrfRequiredForMethods = map[string]bool{
		fasthttp.MethodPost:   true,
		fasthttp.MethodPut:    true,
		fasthttp.MethodPatch:  true,
		fasthttp.MethodDelete: true,
	}
)

type userContext struct {
	cookie  cookie.CSRF
	context string
	isValid bool
	err     error
}

func (c userContext) GetOpenTelemetryAttributes() []attribute.KeyValue {
	v := []attribute.KeyValue{
		attribute.String(semconv.CashTrackCSRFContextKey, c.context),
		attribute.Bool(semconv.CashTrackCSRFIsValidKey, c.isValid),
	}

	if c.err != nil {
		v = append(v, attribute.String(semconv.CashTrackCSRFErrorKey, c.err.Error()))
	}

	return v
}

type RedisHandler struct {
	client *redis.Client
	jwks   jwks.Provider
}

func NewRedisHandler(client *redis.Client, jwksProvider jwks.Provider) *RedisHandler {
	return &RedisHandler{
		client: client,
		jwks:   jwksProvider,
	}
}

// Handler will check each request of defined HTTP methods for CSRF token
// and rotate the new CSRF token as the response.
func (r *RedisHandler) Handler(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		method := string(ctx.Request.Header.Method())

		if method == fasthttp.MethodOptions {
			h(ctx)

			return
		}

		// The token of the session being replaced says nothing about a login or register
		// request, and enforcing it locks the user out: the Redis entry lives 10 minutes
		// while the auth cookies survive until the refresh token expires, so a stale
		// cshtrka cookie is enough to make IsLogged() true with nothing left to validate
		// against. Skipping rotation matters just as much — it runs off the *old* user
		// context and would overwrite the cookie Seed() just wrote for the new session.
		if sessionEstablishingPaths[string(ctx.Path())] {
			h(ctx)

			return
		}

		spanCtx, span := traces.GetTracer().Start(
			traces.FindParentContext(ctx),
			fmt.Sprintf("csrf validate %s %s", ctx.Request.Header.Method(), ctx.URI().PathOriginal()),
			trace.WithAttributes(traces.RequestAttributes(&ctx.Request)...),
		)
		defer span.End()

		userCtx := r.newUserContext(cookie.ReadCSRFCookie(ctx))
		span.SetAttributes(traces.AttributesGetter(userCtx)...)

		// Load-bearing: this returns before h(ctx) forwards to the API, so a 417 proves the
		// request never reached it — that is why the frontend can safely replay a mutating
		// request after one. Do not reorder relative to h(ctx) below.
		if err := r.validateCsrfRequest(spanCtx, userCtx, method); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "invalid")
			span.End()
			slog.Warn("CSRF token validation error", "trace_id", traces.FindTraceId(ctx), "error", err)
			response.ByErrorAndStatus(err, fasthttp.StatusExpectationFailed).Write(ctx)

			return
		}

		span.End()

		h(ctx)

		if userCtx.cookie.Auth.IsLogged() && csrfRequiredForMethods[method] {
			rotateSpanCtx, rotateSpan := traces.GetTracer().Start(
				traces.FindParentContext(ctx),
				fmt.Sprintf("csrf rotate %s %s", ctx.Request.Header.Method(), ctx.URI().PathOriginal()),
				trace.WithAttributes(traces.RequestAttributes(&ctx.Request)...),
				trace.WithAttributes(traces.ResponseAttributes(&ctx.Response)...),
			)
			defer rotateSpan.End()

			newToken, err := r.rotate(rotateSpanCtx, userCtx)
			if err != nil {
				rotateSpan.RecordError(err)
				rotateSpan.SetStatus(codes.Error, "rotate error")
				slog.Error("CSRF token rotation failed, response left as-is",
					"trace_id", traces.FindTraceId(ctx), "error", err)
				csrfRotationFailedTotal.Inc()

				return
			}

			userCtx.cookie.Token = newToken
			userCtx.cookie.WriteCookie(ctx)
		}
	}
}

// RotateTokenHandler configure CSRF cookie for next request validation.
func (r *RedisHandler) RotateTokenHandler(ctx *fasthttp.RequestCtx) {
	spanCtx, span := traces.GetTracer().Start(
		traces.FindParentContext(ctx),
		fmt.Sprintf("csrf rotate handler %s %s", ctx.Request.Header.Method(), ctx.URI().PathOriginal()),
		trace.WithAttributes(traces.RequestAttributes(&ctx.Request)...),
	)
	defer span.End()

	userCtx := r.newUserContext(cookie.ReadCSRFCookie(ctx))
	span.SetAttributes(traces.AttributesGetter(userCtx)...)

	if !userCtx.cookie.Auth.IsLogged() {
		span.SetStatus(codes.Error, "unauthorized")
		userCtx.cookie.WriteCookie(ctx)
		ctx.SetStatusCode(fasthttp.StatusUnauthorized)

		return
	}

	newToken, err := r.rotate(spanCtx, userCtx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "unknown")
		slog.Warn("CSRF token rotation error", "trace_id", traces.FindTraceId(ctx), "error", err)
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)

		return
	}

	userCtx.cookie.Token = newToken
	userCtx.cookie.WriteCookie(ctx)
	ctx.SetStatusCode(fasthttp.StatusOK)
	span.SetStatus(codes.Ok, "")
}

// Seed initialises the CSRF token for a freshly authenticated or refreshed session.
// auth must be passed directly because the access token lives in the response body,
// not in the incoming request cookies.
func (r *RedisHandler) Seed(ctx *fasthttp.RequestCtx, auth cookie.Auth) error {
	if !auth.IsLogged() {
		return nil
	}

	spanCtx, span := traces.GetTracer().Start(
		traces.FindParentContext(ctx),
		fmt.Sprintf("csrf seed %s %s", ctx.Request.Header.Method(), ctx.URI().PathOriginal()),
		trace.WithAttributes(traces.RequestAttributes(&ctx.Request)...),
	)
	defer span.End()

	csrfCookie := cookie.CSRF{Auth: auth}
	userCtx := r.newUserContext(csrfCookie)
	if userCtx.err != nil {
		span.RecordError(userCtx.err)
		span.SetStatus(codes.Error, "invalid token")

		return fmt.Errorf("csrf seed: invalid access token: %w", userCtx.err)
	}

	newToken, err := r.rotate(spanCtx, userCtx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "rotate error")

		return fmt.Errorf("csrf seed: rotate failed: %w", err)
	}

	userCtx.cookie.Token = newToken
	userCtx.cookie.WriteCookie(ctx)

	return nil
}

func (r *RedisHandler) validateCsrfRequest(ctx context.Context, userCtx userContext, method string) error {
	if _, ok := csrfRequiredForMethods[method]; !ok {
		return nil
	}

	if !userCtx.cookie.Auth.IsLogged() {
		return nil
	}

	if userCtx.err != nil {
		return fmt.Errorf("unable to verify with invalid user context: %w", userCtx.err)
	}

	return r.verify(ctx, userCtx)
}

func (r *RedisHandler) rotate(ctx context.Context, userCtx userContext) (string, error) {
	key := fmt.Sprintf("%s:%s", keyPrefix, userCtx.context)

	token := generateNewToken()

	if err := r.client.SetEx(ctx, key, token, tokenTtl).Err(); err != nil {
		csrfRedisUp.Set(0)

		return "", fmt.Errorf("error on writing new token: %w", err)
	}
	csrfRedisUp.Set(1)

	return token, nil
}

// verify checks the request's CSRF token against the one stored in Redis. A missing token
// (redis.Nil) is a real failure and stays a 417. Any other error means Redis is
// unreachable: fail open, since SameSite=Strict auth cookies already block classic CSRF
// and this token is defence-in-depth only.
func (r *RedisHandler) verify(ctx context.Context, userCtx userContext) error {
	key := fmt.Sprintf("%s:%s", keyPrefix, userCtx.context)

	cmd := r.client.Get(ctx, key)
	if err := cmd.Err(); err != nil {
		if errors.Is(err, redis.Nil) {
			csrfRedisUp.Set(1)

			return fmt.Errorf("error on reading token: %w", err)
		}

		csrfRedisUp.Set(0)
		slog.Error("CSRF validation failed open, redis unreachable",
			"trace_id", trace.SpanContextFromContext(ctx).TraceID().String(), "error", err)
		csrfValidationFailedOpenTotal.Inc()

		return nil
	}
	csrfRedisUp.Set(1)

	if strings.Compare(userCtx.cookie.Token, cmd.Val()) != 0 {
		// Do not log the requested/stored token values.
		slog.Warn("CSRF token mismatch",
			"trace_id", trace.SpanContextFromContext(ctx).TraceID().String())

		return fmt.Errorf("invalid CSRF token")
	}

	return nil
}

func generateNewToken() string {
	token, _ := uuid.NewV7()

	return token.String()
}

func (r *RedisHandler) newUserContext(cookie cookie.CSRF) userContext {
	ctx, err := r.getUserContextFromAccessToken(cookie.Auth.AccessToken)
	userCtx := userContext{
		cookie:  cookie,
		context: ctx,
		isValid: true,
	}

	if err != nil {
		userCtx.isValid = false
		userCtx.err = err
	}

	return userCtx
}

// getUserContextFromAccessToken derives the "{sub}:{iat}" CSRF context from the access
// token. Unverified, its claims are attacker-controlled and can be used to rotate or
// poison another user's CSRF entry in Redis.
func (r *RedisHandler) getUserContextFromAccessToken(accessToken string) (string, error) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("JWT decoding recovered from panic", "error", rec)
		}
	}()

	if accessToken == "" {
		return "", fmt.Errorf("access token is empty")
	}

	claims, err := r.parseClaims(accessToken)
	if err != nil {
		return "", err
	}

	return extractUserContext(claims)
}

// parseClaims verifies the token's RS256 signature against the in-memory JWKS key set.
// With no key material loaded at all it fails open to an unverified decode, so the
// gateway keeps working against an HS256-configured API.
//
// Claims validation is intentionally OFF: this handler runs before the proxy, and the
// proxy is what triggers token refresh on a 401 from the API. Rejecting an expired but
// genuine token here would lock the user out until the cookie expires. The API enforces
// expiry downstream.
func (r *RedisHandler) parseClaims(accessToken string) (jwt.MapClaims, error) {
	if r.jwks == nil || !r.jwks.Loaded() {
		slog.Warn("CSRF: no JWKS key material loaded, accepting access token without signature verification")
		csrfSignatureVerificationFailOpenTotal.Inc()

		return parseUnverifiedClaims(accessToken)
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}), jwt.WithoutClaimsValidation())

	// claims is a map, so ParseWithClaims decodes into it in place.
	claims := jwt.MapClaims{}
	if _, err := parser.ParseWithClaims(accessToken, claims, r.keyfunc); err != nil {
		return nil, fmt.Errorf("access token signature verification failed: %w", err)
	}

	return claims, nil
}

// keyfunc resolves the token's "kid" header against the in-memory JWKS set. The token
// never steers the signing method: jwt.WithValidMethods pinned it to RS256 beforehand.
func (r *RedisHandler) keyfunc(token *jwt.Token) (interface{}, error) {
	kid, ok := token.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, fmt.Errorf("access token missing kid header")
	}

	if key, ok := r.jwks.Key(kid); ok {
		return key, nil
	}

	return nil, fmt.Errorf("access token kid not found in current key set")
}

func parseUnverifiedClaims(accessToken string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(accessToken, claims); err != nil {
		return nil, fmt.Errorf("could not parse access token")
	}

	return claims, nil
}

func extractUserContext(claims jwt.MapClaims) (string, error) {
	var userId string
	var issuedAt string

	if u, ok := claims["sub"]; ok {
		userId = strconv.FormatFloat(u.(float64), 'f', 0, 64)
	} else {
		return "", fmt.Errorf("could not extract user id from claims")
	}

	if i, ok := claims["iat"]; ok {
		issuedAt = strconv.FormatFloat(i.(float64), 'f', 0, 64)
	} else {
		return "", fmt.Errorf("could not extract issued at from claims")
	}

	if userId == "" || userId == "0" || issuedAt == "" || issuedAt == "0" {
		return "", fmt.Errorf("could not extract user id or issued at from claims")
	}

	// include iat claim to allow different clients having different CSRF tokens
	return fmt.Sprintf("%s:%s", userId, issuedAt), nil
}
