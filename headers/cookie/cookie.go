package cookie

import (
	"time"

	"github.com/valyala/fasthttp"

	"github.com/cash-track/gateway/config"
)

func newCookie(key, value string, expire time.Time) *fasthttp.Cookie {
	cookie := new(fasthttp.Cookie)
	cookie.SetKey(key)
	cookie.SetValue(value)
	cookie.SetPath("/")
	cookie.SetDomain(config.Global.CookieDomain)
	cookie.SetSecure(config.Global.CookieSecure)
	cookie.SetHTTPOnly(true)
	cookie.SetSameSite(fasthttp.CookieSameSiteStrictMode)
	cookie.SetExpire(expire)

	return cookie
}
