package csrf

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/cash-track/gateway/headers/cookie"
	"github.com/cash-track/gateway/router/response"
	"github.com/cash-track/gateway/traces"
)

const (
	keyPrefix = "CT:csrf"
	tokenTtl  = time.Minute * 10
)

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

func newUserContext(cookie cookie.CSRF) userContext {
	ctx, err := getUserContextFromAccessToken(cookie.Auth.AccessToken)
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

func (c userContext) GetOpenTelemetryAttributes() []attribute.KeyValue {
	v := []attribute.KeyValue{
		attribute.String("ct.csrf.context", c.context),
		attribute.Bool("ct.csrf.is_valid", c.isValid),
	}

	if c.err != nil {
		v = append(v, attribute.String("ct.csrf.error", c.err.Error()))
	}

	return v
}

type RedisHandler struct {
	client *redis.Client
}

func NewRedisHandler(client *redis.Client) *RedisHandler {
	return &RedisHandler{
		client: client,
	}
}

// Handler will check each request of defined HTTP methods for CSRF token
// and rotate the new CSRF token as the response
func (r *RedisHandler) Handler(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		method := string(ctx.Request.Header.Method())

		if method == fasthttp.MethodOptions {
			h(ctx)
			return
		}

		_, span := traces.GetTracer().Start(
			traces.FindParentContext(ctx),
			fmt.Sprintf("csrf validate %s %s", ctx.Request.Header.Method(), ctx.URI().PathOriginal()),
			trace.WithAttributes(traces.RequestAttributes(&ctx.Request)...),
		)
		defer span.End()

		userCtx := newUserContext(cookie.ReadCSRFCookie(ctx))
		span.SetAttributes(traces.AttributesGetter(userCtx)...)

		if err := r.validateCsrfRequest(userCtx, method); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "invalid")
			span.End()
			log.Printf("Error on validating CSRF token: %v", err)
			response.ByErrorAndStatus(err, fasthttp.StatusExpectationFailed).Write(ctx)
			return
		}

		span.End()

		h(ctx)

		if userCtx.cookie.Auth.IsLogged() {
			_, rotateSpan := traces.GetTracer().Start(
				traces.FindParentContext(ctx),
				fmt.Sprintf("csrf rotate %s %s", ctx.Request.Header.Method(), ctx.URI().PathOriginal()),
				trace.WithAttributes(traces.RequestAttributes(&ctx.Request)...),
				trace.WithAttributes(traces.ResponseAttributes(&ctx.Response)...),
			)
			defer rotateSpan.End()

			newToken, err := r.rotate(userCtx)
			if err != nil {
				rotateSpan.RecordError(err)
				rotateSpan.SetStatus(codes.Error, "rotate error")
				log.Printf("Error on rotating CSRF token: %v", err)
				ctx.SetStatusCode(fasthttp.StatusInternalServerError)
				return
			}

			userCtx.cookie.Token = newToken
			userCtx.cookie.WriteCookie(ctx)
		}
	}
}

func (r *RedisHandler) validateCsrfRequest(ctx userContext, method string) error {
	if _, ok := csrfRequiredForMethods[method]; !ok {
		return nil
	}

	if !ctx.cookie.Auth.IsLogged() {
		return nil
	}

	if ctx.err != nil {
		return fmt.Errorf("unable to verify with invalid user context: %w", ctx.err)
	}

	return r.verify(ctx)
}

// RotateTokenHandler configure CSRF cookie for next request validation
func (r *RedisHandler) RotateTokenHandler(ctx *fasthttp.RequestCtx) {
	_, span := traces.GetTracer().Start(
		traces.FindParentContext(ctx),
		fmt.Sprintf("csrf rotate handler %s %s", ctx.Request.Header.Method(), ctx.URI().PathOriginal()),
		trace.WithAttributes(traces.RequestAttributes(&ctx.Request)...),
	)
	defer span.End()

	userCtx := newUserContext(cookie.ReadCSRFCookie(ctx))
	span.SetAttributes(traces.AttributesGetter(userCtx)...)

	if !userCtx.cookie.Auth.IsLogged() {
		span.SetStatus(codes.Error, "unauthorized")
		userCtx.cookie.WriteCookie(ctx)
		ctx.SetStatusCode(fasthttp.StatusUnauthorized)
		return
	}

	newToken, err := r.rotate(userCtx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "unknown")
		log.Printf("Error on rotating CSRF token: %v", err)
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	userCtx.cookie.Token = newToken
	userCtx.cookie.WriteCookie(ctx)
	ctx.SetStatusCode(fasthttp.StatusOK)
	span.SetStatus(codes.Ok, "")
}

func (r *RedisHandler) rotate(ctx userContext) (string, error) {
	key := fmt.Sprintf("%s:%s", keyPrefix, ctx.context)

	token := generateNewToken()

	if err := r.client.SetEx(context.Background(), key, token, tokenTtl).Err(); err != nil {
		return "", fmt.Errorf("error on writing new token: %w", err)
	}

	return token, nil
}

func (r *RedisHandler) verify(ctx userContext) error {
	key := fmt.Sprintf("%s:%s", keyPrefix, ctx.context)

	if cmd := r.client.Get(context.Background(), key); cmd.Err() != nil {
		return fmt.Errorf("error on reading token: %w", cmd.Err())
	} else if strings.Compare(ctx.cookie.Token, cmd.Val()) != 0 {
		log.Printf("CSRF token is invalid: requested %s stored %s", ctx.cookie.Token, cmd.Val())
		return fmt.Errorf("invalid CSRF token")
	}

	return nil
}

func generateNewToken() string {
	token, _ := uuid.NewV7()
	return token.String()
}

func getUserContextFromAccessToken(accessToken string) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("JWT decoding recovered from panic: %v", r)
		}
	}()

	if accessToken == "" {
		return "", fmt.Errorf("access token is empty")
	}

	token, _, err := jwt.NewParser().ParseUnverified(accessToken, jwt.MapClaims{})
	if err != nil || token == nil {
		return "", fmt.Errorf("could not parse access token")
	}

	var claims jwt.MapClaims
	if c, ok := token.Claims.(jwt.MapClaims); ok {
		claims = c
	}

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
