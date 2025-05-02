package traces

import (
	"fmt"
	"strings"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
)

const (
	headerPartsCount = 2
)

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
