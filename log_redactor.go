package golitekit

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

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

// sanitizeCapturedBody is sanitizeLoggedBody for a body of which only a prefix
// was captured. A partial JSON body cannot be parsed for redaction, so it is
// omitted rather than logged unredacted.
func sanitizeCapturedBody(captured []byte, total int64, limit int64, contentType string) string {
	if total > int64(len(captured)) && isLoggableContentType(contentType) && isJSONContentType(contentType) {
		return fmt.Sprintf("[json body omitted: %d bytes exceeds capture limit]", total)
	}
	return sanitizeLoggedBody(captured, limit, contentType)
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

// sensitiveKeyParts mark a key as sensitive when they appear anywhere in its
// normalized form (lowercase, without '_' and '-'), e.g. "userPassword",
// "access_token", "X-Api-Key".
var sensitiveKeyParts = []string{
	"password",
	"passwd",
	"token",
	"secret",
	"authorization",
	"apikey",
	"session",
	"cookie",
	"credential",
	"privatekey",
}

// sensitiveKeysExact are too short to match as substrings without false
// positives (e.g. "author"), so they must equal the normalized key.
var sensitiveKeysExact = []string{"pwd", "auth"}

// isSensitiveKey reports whether values under key are redacted from logged
// JSON bodies and query strings.
func isSensitiveKey(key string) bool {
	normalized := strings.Map(func(r rune) rune {
		if r == '_' || r == '-' {
			return -1
		}
		return unicode.ToLower(r)
	}, key)
	for _, part := range sensitiveKeyParts {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return slices.Contains(sensitiveKeysExact, normalized)
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
	return sanitizeURLWithQuery(u.Path, u.Query())
}

// sanitizeURLWithQuery is sanitizeURL with an already parsed query.
func sanitizeURLWithQuery(path string, values url.Values) string {
	query := sanitizeQuery(values)
	if query == "" {
		return path
	}
	return path + "?" + query
}

// sensitiveAssignmentPattern matches key=value / key: value in error text;
// redactAssignments decides by isSensitiveKey.
var sensitiveAssignmentPattern = regexp.MustCompile(`(?i)\b([a-z0-9_-]+)\s*[:=]\s*("[^"]*"|'[^']*'|(?:bearer|basic|digest|token)\s+[^\s,;]+|[^\s,;]+)`)

func sanitizeErrorMessage(message string, limit int64) string {
	if strings.TrimSpace(message) == "" {
		return ""
	}
	return truncateBody([]byte(redactAssignments(message)), limit)
}

// redactAssignments replaces values of sensitive keys. A match on a
// non-sensitive key only skips the key, so "failed: password=x" is still
// scanned from "password".
func redactAssignments(s string) string {
	var b strings.Builder
	for {
		loc := sensitiveAssignmentPattern.FindStringSubmatchIndex(s)
		if loc == nil {
			break
		}
		key := s[loc[2]:loc[3]]
		if !isSensitiveKey(key) {
			b.WriteString(s[:loc[3]])
			s = s[loc[3]:]
			continue
		}
		b.WriteString(s[:loc[0]])
		b.WriteString(key)
		b.WriteString("=[REDACTED]")
		s = s[loc[1]:]
	}
	b.WriteString(s)
	return b.String()
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
