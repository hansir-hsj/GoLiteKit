package golitekit

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hansir-hsj/GoLiteKit/env"
	"github.com/hansir-hsj/GoLiteKit/logger"
)

// DefaultLogBodyLimit is the max bytes logged for request/response bodies.
const DefaultLogBodyLimit int64 = 4096

// sensitiveKeys are redacted from logged JSON bodies (case-insensitive).
var sensitiveKeys = []string{"password", "token", "secret", "authorization"}

// LoggerOptions configures the logger middleware.
type LoggerOptions struct {
	LogRequestBody  bool
	LogResponseBody bool
	MaxBodyBytes    int64
}

// LoggerAsMiddleware logs each request and its outcome using logInst.
// panicInst is optional; pass nil to skip panic logging.
func LoggerAsMiddleware(logInst logger.Logger, panicInst *logger.PanicLogger, opts ...LoggerOptions) Middleware {
	var opt LoggerOptions
	useEnv := true
	if len(opts) > 0 {
		opt = opts[0]
		useEnv = false
	}
	if opt.MaxBodyBytes <= 0 {
		opt.MaxBodyBytes = DefaultLogBodyLimit
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) (rerr error) {
			gcx := GetContext(ctx)

			rw := newResponseCapture(w)
			defer func() {
				logReqBody := opt.LogRequestBody
				logRespBody := opt.LogResponseBody
				if useEnv {
					logReqBody = env.LogRequestBody()
					logRespBody = env.LogResponseBody()
				}

				if logReqBody && r.Method != http.MethodGet && r.Method != http.MethodDelete {
					if gcx != nil && len(gcx.RawBody) > 0 && isLoggableContentType(r.Header.Get("Content-Type")) {
						logger.AddInfo(ctx, "request", sanitizeBody(gcx.RawBody, opt.MaxBodyBytes))
					}
				}

				if logRespBody && len(rw.body) > 0 {
					logger.AddInfo(ctx, "response", truncateBody(rw.body, opt.MaxBodyBytes))
				}

				logger.AddInfo(ctx, "status", rw.statusCode)

				if rerr != nil {
					if appErr, ok := rerr.(*AppError); ok {
						logger.AddInfo(ctx, "err_code", appErr.Code)
						logger.AddInfo(ctx, "err_message", appErr.Message)
						if appErr.Internal != nil {
							logger.AddInfo(ctx, "err_internal", appErr.Internal.Error())
						}
					}
					if logInst != nil {
						logInst.Warning(ctx, "request completed with error")
					}
				} else {
					if logInst != nil {
						logInst.Info(ctx, "succ")
					}
				}

				rw.resetForPool()
				responseCapturePool.Put(rw)
			}()

			if gcx != nil {
				gcx.responseWriter = rw
				gcx.SetContextOptions(withLogger(logInst), withPanicLogger(panicInst))
			}

			logger.AddInfo(ctx, "method", r.Method)
			logger.AddInfo(ctx, "url", r.URL.String())

			rerr = next(ctx, rw, r)
			return
		}
	}
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
	truncated := truncateBody(body, limit)
	return redactSensitiveKeys(truncated)
}

// redactSensitiveKeys replaces values of sensitive keys in JSON strings.
func redactSensitiveKeys(s string) string {
	// Try to parse as JSON for proper redaction
	var data map[string]any
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return s
	}
	redactMap(data)
	out, err := json.Marshal(data)
	if err != nil {
		return s
	}
	return string(out)
}

func redactMap(m map[string]any) {
	for k, v := range m {
		if isSensitiveKey(k) {
			m[k] = "[REDACTED]"
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			redactMap(nested)
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

