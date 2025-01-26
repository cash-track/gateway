package csrf

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-redis/redismock/v9"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/headers/cookie"
)

func TestHandler(t *testing.T) {
	accessToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": 123987,
		"iat": 987654321,
	}).SignedString([]byte("asd"))

	for name, test := range map[string]struct {
		request      *fasthttp.RequestCtx
		setup        func(mock redismock.ClientMock)
		expectPass   bool
		expectStatus int
	}{
		"TokenValidForPost": {
			request: func() *fasthttp.RequestCtx {
				ctx := fasthttp.RequestCtx{}
				ctx.Request.Header.SetMethod(fasthttp.MethodPost)
				ctx.Request.Header.SetCookie(cookie.CsrfTokenCookieName, "csrf_token")
				ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, accessToken)
				return &ctx
			}(),
			setup: func(mock redismock.ClientMock) {
				key := fmt.Sprintf("%s:%d:%d", keyPrefix, 123987, 987654321)
				mock.ExpectGet(key).SetVal("csrf_token")
				mock.CustomMatch(func(expected, actual []interface{}) error {
					assert.NotNil(t, actual)
					if s, ok := actual[1].(string); ok {
						assert.IsType(t, "", s)
					}
					return nil
				}).ExpectSetEx(key, nil, 0).SetVal("token_1")
			},
			expectPass: true,
		},
		"TokenInvalidForPost": {
			request: func() *fasthttp.RequestCtx {
				ctx := fasthttp.RequestCtx{}
				ctx.Request.Header.SetMethod(fasthttp.MethodPost)
				ctx.Request.Header.SetCookie(cookie.CsrfTokenCookieName, "csrf_token_invalid")
				ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, accessToken)
				return &ctx
			}(),
			setup: func(mock redismock.ClientMock) {
				key := fmt.Sprintf("%s:%d:%d", keyPrefix, 123987, 987654321)
				mock.ExpectGet(key).SetVal("csrf_token")
			},
			expectPass:   false,
			expectStatus: fasthttp.StatusExpectationFailed,
		},
		"SkippedForOptions": {
			request: func() *fasthttp.RequestCtx {
				ctx := fasthttp.RequestCtx{}
				ctx.Request.Header.SetMethod(fasthttp.MethodOptions)
				return &ctx
			}(),
			setup: func(mock redismock.ClientMock) {
			},
			expectPass: true,
		},
		"ValidationSkippedForGet": {
			request: func() *fasthttp.RequestCtx {
				ctx := fasthttp.RequestCtx{}
				ctx.Request.Header.SetMethod(fasthttp.MethodGet)
				ctx.Request.Header.SetCookie(cookie.CsrfTokenCookieName, "csrf_token")
				ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, accessToken)
				return &ctx
			}(),
			setup: func(mock redismock.ClientMock) {
				key := fmt.Sprintf("%s:%d:%d", keyPrefix, 123987, 987654321)
				mock.CustomMatch(func(expected, actual []interface{}) error {
					assert.NotNil(t, actual)
					if s, ok := actual[1].(string); ok {
						assert.IsType(t, "", s)
					}
					return nil
				}).ExpectSetEx(key, nil, 0).SetVal("token_1")
			},
			expectPass: true,
		},
		"SkippedForGuest": {
			request: func() *fasthttp.RequestCtx {
				ctx := fasthttp.RequestCtx{}
				ctx.Request.Header.SetMethod(fasthttp.MethodPost)
				return &ctx
			}(),
			setup: func(mock redismock.ClientMock) {
			},
			expectPass: true,
		},
		"FailForInvalidAccessToken": {
			request: func() *fasthttp.RequestCtx {
				ctx := fasthttp.RequestCtx{}
				ctx.Request.Header.SetMethod(fasthttp.MethodPost)
				ctx.Request.Header.SetCookie(cookie.CsrfTokenCookieName, "csrf_token")
				ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, "123")
				return &ctx
			}(),
			setup: func(mock redismock.ClientMock) {
			},
			expectPass: false,
		},
		"VerifyError": {
			request: func() *fasthttp.RequestCtx {
				ctx := fasthttp.RequestCtx{}
				ctx.Request.Header.SetMethod(fasthttp.MethodPost)
				ctx.Request.Header.SetCookie(cookie.CsrfTokenCookieName, "csrf_token")
				ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, accessToken)
				return &ctx
			}(),
			setup: func(mock redismock.ClientMock) {
				key := fmt.Sprintf("%s:%d:%d", keyPrefix, 123987, 987654321)
				mock.ExpectGet(key).SetErr(errors.New("broken pipe"))
			},
			expectPass:   false,
			expectStatus: fasthttp.StatusExpectationFailed,
		},
		"RotateError": {
			request: func() *fasthttp.RequestCtx {
				ctx := fasthttp.RequestCtx{}
				ctx.Request.Header.SetMethod(fasthttp.MethodPost)
				ctx.Request.Header.SetCookie(cookie.CsrfTokenCookieName, "csrf_token")
				ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, accessToken)
				return &ctx
			}(),
			setup: func(mock redismock.ClientMock) {
				key := fmt.Sprintf("%s:%d:%d", keyPrefix, 123987, 987654321)
				mock.ExpectGet(key).SetVal("csrf_token")
				mock.CustomMatch(func(expected, actual []interface{}) error {
					assert.NotNil(t, actual)
					if s, ok := actual[1].(string); ok {
						assert.IsType(t, "", s)
					}
					return nil
				}).ExpectSetEx(key, nil, 0).SetErr(errors.New("broken pipe"))
			},
			expectPass:   true,
			expectStatus: fasthttp.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			client, mock := redismock.NewClientMock()

			test.setup(mock)

			handlersExecuted := false

			handler := NewRedisHandler(client)
			handler.Handler(func(ctx *fasthttp.RequestCtx) {
				handlersExecuted = true
			})(test.request)

			assert.Equal(t, test.expectPass, handlersExecuted)
			assert.NotEqual(t, string(test.request.Response.Header.PeekCookie(cookie.CsrfTokenCookieName)), "csrf_token")
			if test.expectStatus > 0 {
				assert.Equal(t, test.expectStatus, test.request.Response.StatusCode())
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestRotateTokenHandler(t *testing.T) {
	accessToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": 123987,
		"iat": 987654321,
	}).SignedString([]byte("asd"))

	for name, test := range map[string]struct {
		request      *fasthttp.RequestCtx
		setup        func(mock redismock.ClientMock)
		expectRotate bool
		expectStatus int
	}{
		"Rotate": {
			request: func() *fasthttp.RequestCtx {
				ctx := fasthttp.RequestCtx{}
				ctx.Request.Header.SetCookie(cookie.CsrfTokenCookieName, "csrf_token")
				ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, accessToken)
				return &ctx
			}(),
			setup: func(mock redismock.ClientMock) {
				key := fmt.Sprintf("%s:%d:%d", keyPrefix, 123987, 987654321)
				mock.CustomMatch(func(expected, actual []interface{}) error {
					assert.NotNil(t, actual)
					if s, ok := actual[1].(string); ok {
						assert.IsType(t, "", s)
					}
					return nil
				}).ExpectSetEx(key, nil, 0).SetVal("token_1")
			},
			expectRotate: true,
			expectStatus: fasthttp.StatusOK,
		},
		"Guest": {
			request: func() *fasthttp.RequestCtx {
				ctx := fasthttp.RequestCtx{}
				return &ctx
			}(),
			setup: func(mock redismock.ClientMock) {
			},
			expectRotate: false,
			expectStatus: fasthttp.StatusUnauthorized,
		},
		"RotateError": {
			request: func() *fasthttp.RequestCtx {
				ctx := fasthttp.RequestCtx{}
				ctx.Request.Header.SetCookie(cookie.CsrfTokenCookieName, "csrf_token")
				ctx.Request.Header.SetCookie(cookie.AccessTokenCookieName, accessToken)
				return &ctx
			}(),
			setup: func(mock redismock.ClientMock) {
				key := fmt.Sprintf("%s:%d:%d", keyPrefix, 123987, 987654321)
				mock.CustomMatch(func(expected, actual []interface{}) error {
					assert.NotNil(t, actual)
					if s, ok := actual[1].(string); ok {
						assert.IsType(t, "", s)
					}
					return nil
				}).ExpectSetEx(key, nil, 0).SetErr(errors.New("broken pipe"))
			},
			expectRotate: false,
			expectStatus: fasthttp.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			client, mock := redismock.NewClientMock()

			test.setup(mock)

			handler := NewRedisHandler(client)
			handler.RotateTokenHandler(test.request)

			if test.expectRotate {
				assert.NotEqual(t, string(test.request.Response.Header.PeekCookie(cookie.CsrfTokenCookieName)), "csrf_token")
			}

			assert.Equal(t, test.expectStatus, test.request.Response.StatusCode())

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestGetUserContextFromAccessToken(t *testing.T) {
	for name, test := range map[string]struct {
		token         string
		expectContext string
		expectError   bool
		expectPanic   bool
	}{
		"OK": {
			token: func() string {
				s, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
					"sub": 123987,
					"iat": 987654321,
				}).SignedString([]byte("asd"))
				return s
			}(),
			expectContext: "123987:987654321",
		},
		"Empty": {
			token:       "",
			expectError: true,
		},
		"Invalid": {
			token:       "not jwt token",
			expectError: true,
		},
		"NoClaims": {
			token: func() string {
				s, _ := jwt.New(jwt.SigningMethodHS256).SignedString([]byte("asd"))
				return s
			}(),
			expectError: true,
		},
		"NoUserId": {
			token: func() string {
				s, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
					"iat": 987654321,
				}).SignedString([]byte("asd"))
				return s
			}(),
			expectError: true,
		},
		"NoIssuedTimestamp": {
			token: func() string {
				s, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
					"sub": 123987,
				}).SignedString([]byte("asd"))
				return s
			}(),
			expectError: true,
		},
		"EmptyUserId": {
			token: func() string {
				s, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
					"sub": 0,
					"iat": 987654321,
				}).SignedString([]byte("asd"))
				return s
			}(),
			expectError: true,
		},
		"EmptyIssuedTimestamp": {
			token: func() string {
				s, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
					"sub": 123987,
					"iat": 0,
				}).SignedString([]byte("asd"))
				return s
			}(),
			expectError: true,
		},
		"ClaimsPanic": {
			token: func() string {
				s, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
					"sub": "",
					"iat": "",
				}).SignedString([]byte("asd"))
				return s
			}(),
			expectPanic: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if test.expectPanic {
				_, _ = getUserContextFromAccessToken(test.token)
				return
			}
			ctx, err := getUserContextFromAccessToken(test.token)
			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expectContext, ctx)
		})
	}
}
