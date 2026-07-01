package traces

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

const (
	headerPartsCount   = 2
	bodyCaptureMaxSize = 4096
	bodyTruncatedNote  = "...(truncated)"
)

// sensitiveBodyFieldSubstrings matches JSON field names (case-insensitive, substring)
// that must be redacted from captured request/response bodies. Substring matching
// (rather than an exact-name set) also catches variants like passwordConfirmation
// or newPasswordConfirmation without needing to enumerate every field name.
var sensitiveBodyFieldSubstrings = []string{
	"password",
	"token",
	"secret",
	"apikey",
}

type OpenTelemetryAttributesGetter interface {
	GetOpenTelemetryAttributes() []attribute.KeyValue
}

func Attributes(attributes ...attribute.KeyValue) []attribute.KeyValue {
	return attributes
}

func AttributesGetter(getters ...OpenTelemetryAttributesGetter) []attribute.KeyValue {
	var result []attribute.KeyValue
	for _, getter := range getters {
		result = append(result, getter.GetOpenTelemetryAttributes()...)
	}

	return result
}

func MergeAttributes(attributes ...[]attribute.KeyValue) []attribute.KeyValue {
	var result []attribute.KeyValue
	for _, attr := range attributes {
		result = append(result, attr...)
	}

	return result
}

func RequestAttributes(req *fasthttp.Request) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(string(req.Header.Method())),
		semconv.HTTPRequestSizeKey.Int(req.Header.ContentLength()),
		semconv.URLFull(string(req.URI().FullURI())),
		semconv.HTTPRouteKey.String(string(req.URI().Path())),
		semconv.UserAgentNameKey.String(string(req.Header.UserAgent())),
		semconv.UserAgentOriginalKey.String(string(req.Header.UserAgent())),
		attribute.String("http.request.headers", SanitizeHTTPHeaders(req.Header.String())),
	}
}

func ResponseAttributes(res *fasthttp.Response) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.HTTPResponseStatusCode(res.StatusCode()),
		semconv.HTTPResponseSizeKey.Int(res.Header.ContentLength()),
		attribute.String("http.response.headers", SanitizeHTTPHeaders(res.Header.String())),
	}
}

// RequestBodyAttribute captures a redacted, size-capped copy of the request body as a
// span attribute. Only attempted for JSON bodies; anything else (multipart uploads,
// empty bodies) is replaced with a placeholder so binary data never reaches Tempo.
func RequestBodyAttribute(req *fasthttp.Request) attribute.KeyValue {
	return attribute.String("http.request.body", SanitizeJSONBody(req.Header.ContentType(), req.Body()))
}

// ResponseBodyAttribute captures a redacted, size-capped copy of the response body as a
// span attribute. See RequestBodyAttribute.
func ResponseBodyAttribute(res *fasthttp.Response) attribute.KeyValue {
	return attribute.String("http.response.body", SanitizeJSONBody(res.Header.ContentType(), res.Body()))
}

// SanitizeJSONBody redacts sensitive fields from a JSON body and caps its size.
// The body is fully parsed and re-marshalled before truncation so a size cutoff
// can never land mid-field and leak part of a secret.
func SanitizeJSONBody(contentType, body []byte) string {
	if len(body) == 0 {
		return "(empty)"
	}

	if !bytes.Contains(bytes.ToLower(contentType), []byte("application/json")) {
		return fmt.Sprintf("(omitted: content-type %s)", string(contentType))
	}

	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "(omitted: body is not valid JSON)"
	}

	redacted, err := json.Marshal(redactJSONValue(parsed))
	if err != nil {
		return "(omitted: failed to re-marshal redacted body)"
	}

	if len(redacted) > bodyCaptureMaxSize {
		return string(redacted[:bodyCaptureMaxSize]) + bodyTruncatedNote
	}

	return string(redacted)
}

func redactJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, val := range v {
			if isSensitiveBodyField(key) {
				result[key] = "***"
			} else {
				result[key] = redactJSONValue(val)
			}
		}

		return result
	case []any:
		result := make([]any, len(v))
		for i, val := range v {
			result[i] = redactJSONValue(val)
		}

		return result
	default:
		return v
	}
}

func isSensitiveBodyField(key string) bool {
	key = strings.ToLower(key)
	for _, substr := range sensitiveBodyFieldSubstrings {
		if strings.Contains(key, substr) {
			return true
		}
	}

	return false
}

func SanitizeHTTPHeaders(raw string) string {
	// Define sensitive headers (case-insensitive)
	sensitiveHeaders := map[string]struct{}{
		"cookie":             {},
		"set-cookie":         {},
		"authorization":      {},
		"x-api-key":          {},
		"api-key":            {},
		"access-token":       {},
		"refresh-token":      {},
		"private-token":      {},
		"session-token":      {},
		"password":           {},
		"client-secret":      {},
		"proxy-authenticate": {},
		"www-authenticate":   {},
	}

	// Split raw string into lines
	lines := strings.Split(raw, "\n")
	var result []string

	// Process each line
	for _, line := range lines {
		// Skip empty lines
		if len(strings.TrimSpace(line)) == 0 {
			result = append(result, line)

			continue
		}

		// Split header line into name and value
		parts := strings.SplitN(line, ":", headerPartsCount)
		if len(parts) != headerPartsCount {
			result = append(result, line)

			continue
		}

		headerName := strings.ToLower(strings.TrimSpace(parts[0]))
		if _, isSensitive := sensitiveHeaders[headerName]; isSensitive {
			// Replace sensitive header value with asterisks
			result = append(result, fmt.Sprintf("%s: ***", parts[0]))
		} else {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}
