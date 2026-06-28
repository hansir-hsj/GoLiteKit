package golitekit

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// sensitiveKeys are redacted from logged JSON bodies and query strings.
var sensitiveKeys = []string{
	"password",
	"passwd",
	"pwd",
	"token",
	"access_token",
	"refresh_token",
	"secret",
	"client_secret",
	"authorization",
	"auth",
	"api_key",
	"apikey",
	"session",
	"cookie",
	"credential",
	"private_key",
}

// truncateBody returns body as string, truncated to limit bytes.
func truncateBody(body []byte, limit int64) string {
	if int64(len(body)) <= limit {
		return string(body)
	}
	return string(body[:limit]) + "...(truncated)"
}

// sanitizeBody truncates and redacts sensitive JSON keys.
func sanitizeBody(body []byte, limit int64) string {
	return sanitizeLoggedBody(body, limit, "application/json")
}

func sanitizeLoggedBody(body []byte, limit int64, contentType string) string {
	if !isLoggableContentType(contentType) {
		return "[non-loggable body omitted]"
	}
	if isJSONContentType(contentType) {
		redacted, ok := redactJSONBody(body)
		if !ok {
			return "[invalid json body omitted]"
		}
		return truncateBody(redacted, limit)
	}
	return truncateBody(body, limit)
}

// redactSensitiveKeys replaces values of sensitive keys in JSON strings.
func redactSensitiveKeys(s string) string {
	out, ok := redactJSONBody([]byte(s))
	if !ok {
		return s
	}
	return string(out)
}

func redactJSONBody(body []byte) ([]byte, bool) {
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, false
	}
	redactJSONValue(data)
	out, err := json.Marshal(data)
	if err != nil {
		return nil, false
	}
	return out, true
}

func redactJSONValue(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if isSensitiveKey(key) {
				v[key] = "[REDACTED]"
				continue
			}
			redactJSONValue(child)
		}
	case []any:
		for _, child := range v {
			redactJSONValue(child)
		}
	}
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if lower == s {
			return true
		}
	}
	return false
}

func sanitizeQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	safe := make(url.Values, len(values))
	for key, vals := range values {
		if isSensitiveKey(key) {
			safe[key] = []string{"[REDACTED]"}
			continue
		}
		safe[key] = append([]string(nil), vals...)
	}
	return safe.Encode()
}

func sanitizeURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	query := sanitizeQuery(u.Query())
	if query == "" {
		return u.Path
	}
	return u.Path + "?" + query
}

var sensitiveAssignmentPattern = regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|access_token|refresh_token|secret|client_secret|authorization|auth|api_key|apikey|session|cookie|credential|private_key)\b\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;]+(?:\s+[^\s,;]+)?)`)

func sanitizeErrorMessage(message string, limit int64) string {
	if strings.TrimSpace(message) == "" {
		return ""
	}
	redacted := sensitiveAssignmentPattern.ReplaceAllString(message, "$1=[REDACTED]")
	return truncateBody([]byte(redacted), limit)
}

func isJSONContentType(ct string) bool {
	if ct == "" {
		return true
	}
	lower := strings.ToLower(ct)
	return strings.Contains(lower, "application/json") || strings.Contains(lower, "+json")
}

// isLoggableContentType returns false for binary/multipart content types.
func isLoggableContentType(ct string) bool {
	if ct == "" {
		return true
	}
	lower := strings.ToLower(ct)
	if strings.Contains(lower, "multipart/") ||
		strings.Contains(lower, "application/octet-stream") ||
		strings.Contains(lower, "image/") ||
		strings.Contains(lower, "audio/") ||
		strings.Contains(lower, "video/") {
		return false
	}
	return true
}
