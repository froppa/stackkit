package configkit

import (
	"fmt"
	"strings"
)

var secretWords = []string{"password", "secret", "token", "apikey", "key", "dsn", "cookie", "bearer"}

// Redact masks secret-looking values within v for safe logging/display.
// The key parameter can be used for future, key-specific redaction nuances.
func Redact(_ string, v any) any {
	return redact(normalize(v))
}

func redact(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if isSecretKey(k) {
				out[k] = "***"
				continue
			}
			out[k] = redact(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redact(val)
		}
		return out
	default:
		return t
	}
}

func isSecretKey(k string) bool {
	low := strings.ToLower(k)
	for _, w := range secretWords {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

func normalize(v any) any {
	switch t := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[asString(k)] = normalize(val)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = normalize(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalize(val)
		}
		return out
	default:
		return t
	}
}

func asString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	default:
		return fmt.Sprint(s)
	}
}
