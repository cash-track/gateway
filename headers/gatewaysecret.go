package headers

// WriteGatewaySecret marks the request as gateway-forwarded. No-op when secret is empty.
func WriteGatewaySecret(h headerWriter, secret string) {
	if secret != "" {
		h.Set(XGatewaySecret, secret)
	}
}
