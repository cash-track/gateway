package headers

// headerWriter is satisfied by both *fasthttp.RequestHeader and *fasthttp.ResponseHeader.
type headerWriter interface {
	Set(key, value string)
}

// WriteGatewayVersion sets the gateway build provenance headers, omitting either one
// entirely when its source value is empty.
func WriteGatewayVersion(h headerWriter, gitTag, gitSha string) {
	if gitTag != "" {
		h.Set(XCtGatewayVersion, gitTag)
	}

	if gitSha != "" {
		h.Set(XCtGatewaySha, gitSha)
	}
}
