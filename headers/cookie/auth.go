package cookie

import (
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

func (a Auth) WriteCookie(ctx *fasthttp.RequestCtx) {
	if !a.IsLogged() {
		ctx.Response.Header.SetCookie(newCookie(AccessTokenCookieName, "", fasthttp.CookieExpireDelete))
		ctx.Response.Header.SetCookie(newCookie(RefreshTokenCookieName, "", fasthttp.CookieExpireDelete))

		return
	}

	ctx.Response.Header.SetCookie(newCookie(AccessTokenCookieName, a.AccessToken, a.GetRefreshTokenExpireDate()))
	ctx.Response.Header.SetCookie(newCookie(RefreshTokenCookieName, a.RefreshToken, a.GetRefreshTokenExpireDate()))
}

func (a Auth) IsLogged() bool {
	return a.AccessToken != ""
}

func (a Auth) CanRefresh() bool {
	return a.RefreshToken != ""
}

func (a Auth) GetRefreshTokenExpireDate() time.Time {
	t, _ := time.Parse(time.RFC3339, a.RefreshTokenExpiredAt)

	return t
}

func (a Auth) GetOpenTelemetryAttributes() []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Bool(semconv.CashTrackAuthIsLoggedKey, a.IsLogged()),
		attribute.Bool(semconv.CashTrackAuthCanRefreshKey, a.CanRefresh()),
		attribute.String(semconv.CashTrackAuthAccessTokenExpireAtKey, a.AccessTokenExpiredAt),
		attribute.String(semconv.CashTrackAuthRefreshTokenExpireAtKey, a.RefreshTokenExpiredAt),
	}
}
