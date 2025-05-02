package traces

import (
	"testing"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
)

type MockAttributesGetter struct {
	Attrs []attribute.KeyValue
}

func (m MockAttributesGetter) GetOpenTelemetryAttributes() []attribute.KeyValue {
	return m.Attrs
}

func TestAttributes(t *testing.T) {
	attr1 := attribute.String("key1", "value1")
	attr2 := attribute.Int("key2", 42)

	result := Attributes(attr1, attr2)

	if len(result) != 2 {
		t.Errorf("Expected 2 attributes, got %d", len(result))
	}
	if result[0] != attr1 || result[1] != attr2 {
		t.Error("Attributes not properly stored")
	}
}

func TestAttributesGetter(t *testing.T) {
	mock1 := MockAttributesGetter{
		Attrs: []attribute.KeyValue{
			attribute.String("key1", "value1"),
		},
	}
	mock2 := MockAttributesGetter{
		Attrs: []attribute.KeyValue{
			attribute.Int("key2", 42),
		},
	}

	result := AttributesGetter(&mock1, &mock2)

	if len(result) != 2 {
		t.Errorf("Expected 2 attributes, got %d", len(result))
	}
	if result[0] != mock1.Attrs[0] || result[1] != mock2.Attrs[0] {
		t.Error("Attributes not properly collected from getters")
	}
}

func TestMergeAttributes(t *testing.T) {
	attr1 := []attribute.KeyValue{attribute.String("key1", "value1")}
	attr2 := []attribute.KeyValue{attribute.Int("key2", 42)}

	result := MergeAttributes(attr1, attr2)

	if len(result) != 2 {
		t.Errorf("Expected 2 attributes, got %d", len(result))
	}
	if result[0] != attr1[0] || result[1] != attr2[0] {
		t.Error("Attributes not properly merged")
	}
}

func TestRequestAttributes(t *testing.T) {
	req := &fasthttp.Request{}
	req.Header.SetMethod("POST")
	req.Header.SetRequestURI("http://example.com/test")
	req.Header.Set("User-Agent", "test-agent")
	req.Header.SetContentLength(100)

	attrs := RequestAttributes(req)

	if len(attrs) != 7 {
		t.Errorf("Expected 7 attributes, got %d", len(attrs))
	}

	expectedMethod := string(req.Header.Method())
	if attrs[0].Value.AsString() != expectedMethod {
		t.Errorf("Expected method %s, got %s", expectedMethod, attrs[0].Value.AsString())
	}
}

func TestResponseAttributes(t *testing.T) {
	res := &fasthttp.Response{}
	res.SetStatusCode(200)
	res.Header.SetContentLength(150)

	attrs := ResponseAttributes(res)

	if len(attrs) != 3 {
		t.Errorf("Expected 3 attributes, got %d", len(attrs))
	}

	if attrs[0].Value.AsInt64() != 200 {
		t.Errorf("Expected status code 200, got %d", attrs[0].Value.AsInt64())
	}
}

func TestSanitizeHTTPHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "sensitive headers are masked",
			input: `Host: example.com
Authorization: Bearer token123
Cookie: session=abc123
Content-Type: application/json
X-API-Key: secret-key
Set-Cookie: theme=dark`,
			expected: `Host: example.com
Authorization: ***
Cookie: ***
Content-Type: application/json
X-API-Key: ***
Set-Cookie: ***`,
		},
		{
			name: "empty lines are preserved",
			input: `Host: example.com

Authorization: Bearer token123

Cookie: session=abc123`,
			expected: `Host: example.com

Authorization: ***

Cookie: ***`,
		},
		{
			name: "malformed headers are untouched",
			input: `Host: example.com
Invalid-Header-Line
Authorization: Bearer token123`,
			expected: `Host: example.com
Invalid-Header-Line
Authorization: ***`,
		},
		{
			name:     "empty input returns empty",
			input:    "",
			expected: "",
		},
		{
			name: "case insensitive header matching",
			input: `HOST: example.com
AUTHORIZATION: Bearer token123
cookie: session=abc123`,
			expected: `HOST: example.com
AUTHORIZATION: ***
cookie: ***`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeHTTPHeaders(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeHTTPHeaders() = %v, want %v", result, tt.expected)
			}
		})
	}
}
