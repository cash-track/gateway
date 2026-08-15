package cookie

import (
	"fmt"
	"time"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"

	"github.com/cash-track/gateway/traces/semconv"
)

const (
	AccessTokenCookieName  = "cshtrka"
	RefreshTokenCookieName = "cshtrkr"
)

type Auth struct {
	AccessToken           string `json:"accessToken,omitempty"`
	AccessTokenExpiredAt  string `json:"accessTokenExpiredAt,omitempty"`
	RefreshToken          string `json:"refreshToken,omitempty"`
	RefreshTokenExpiredAt string `json:"refreshTokenExpiredAt,omitempty"`
}

func ReadAuthCookie(ctx *fasthttp.RequestCtx) Auth {
	auth := Auth{}

	if val := ctx.Request.Header.Cookie(AccessTokenCookieName); val != nil {
		auth.AccessToken = string(val)
	}

	if val := ctx.Request.Header.Cookie(RefreshTokenCookieName); val != nil {
		auth.RefreshToken = string(val)
	}

	return auth
}

func (a Auth) WriteCookie(ctx *fasthttp.RequestCtx) error {
	if !a.IsLogged() {
		ctx.Response.Header.SetCookie(newCookie(AccessTokenCookieName, "", fasthttp.CookieExpireDelete))
		ctx.Response.Header.SetCookie(newCookie(RefreshTokenCookieName, "", fasthttp.CookieExpireDelete))

		return nil
	}

	expireAt, err := a.GetRefreshTokenExpireDate()
	if err != nil {
		return fmt.Errorf("refresh token expiry: %w", err)
	}

	ctx.Response.Header.SetCookie(newCookie(AccessTokenCookieName, a.AccessToken, expireAt))
	ctx.Response.Header.SetCookie(newCookie(RefreshTokenCookieName, a.RefreshToken, expireAt))

	return nil
}

func (a Auth) IsLogged() bool {
	return a.AccessToken != ""
}

func (a Auth) CanRefresh() bool {
	return a.RefreshToken != ""
}

func (a Auth) GetRefreshTokenExpireDate() (time.Time, error) {
	t, err := time.Parse(time.RFC3339, a.RefreshTokenExpiredAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse refresh token expiry %q: %w", a.RefreshTokenExpiredAt, err)
	}

	if t.Before(time.Now()) {
		return time.Time{}, fmt.Errorf("refresh token expiry %q is in the past", a.RefreshTokenExpiredAt)
	}

	return t, nil
}

func (a Auth) GetOpenTelemetryAttributes() []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Bool(semconv.CashTrackAuthIsLoggedKey, a.IsLogged()),
		attribute.Bool(semconv.CashTrackAuthCanRefreshKey, a.CanRefresh()),
		attribute.String(semconv.CashTrackAuthAccessTokenExpireAtKey, a.AccessTokenExpiredAt),
		attribute.String(semconv.CashTrackAuthRefreshTokenExpireAtKey, a.RefreshTokenExpiredAt),
	}
}
