package semconv

const (
	CashTrackCaptchaSuccessKey     = "ct.captcha.success"
	CashTrackCaptchaChallengeTSKey = "ct.captcha.challenge_ts"
	CashTrackCaptchaHostnameKey    = "ct.captcha.hostname"
	CashTrackCaptchaScoreKey       = "ct.captcha.score"
	CashTrackCaptchaActionKey      = "ct.captcha.action"
	CashTrackCaptchaErrorCodesKey  = "ct.captcha.error_codes"

	CashTrackAuthIsLoggedKey             = "ct.auth.is_logged"
	CashTrackAuthCanRefreshKey           = "ct.auth.can_refresh"
	CashTrackAuthAccessTokenExpireAtKey  = "ct.auth.access_token_expire_at"
	CashTrackAuthRefreshTokenExpireAtKey = "ct.auth.refresh_token_expire_at"

	CashTrackCSRFContextKey = "ct.csrf.context"
	CashTrackCSRFIsValidKey = "ct.csrf.is_valid"
	CashTrackCSRFErrorKey   = "ct.csrf.error"
)
