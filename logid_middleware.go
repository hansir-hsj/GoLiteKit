package golitekit

import (
	"context"
	"net/http"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

// DefaultLogIDHeader is the request and response header that carries the log ID.
const DefaultLogIDHeader = "X-Request-ID"

// maxLogIDLength bounds log IDs accepted from request headers.
const maxLogIDLength = 64

// LogIDOptions configures LogIDMiddleware. By default a new ID is generated
// per request and echoed in the X-Request-ID response header.
type LogIDOptions struct {
	// Header is the request/response header name; empty means X-Request-ID.
	Header string
	// TrustIncoming reuses a valid incoming ID. Enable only behind a gateway
	// that always sets the header.
	TrustIncoming bool
	// OmitResponseHeader stops the log ID from being written to the response header.
	OmitResponseHeader bool
}

// LogIDMiddleware sets the request's log ID and logs it as "logid". A valid
// incoming ID (1-64 chars of [A-Za-z0-9._:-]) is reused with TrustIncoming,
// otherwise logged as "upstream_request_id".
func LogIDMiddleware(opts ...LogIDOptions) Middleware {
	var opt LogIDOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	header := opt.Header
	if header == "" {
		header = DefaultLogIDHeader
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			// Both are no-ops under Router, whose request context already
			// carries them; they make the middleware usable on its own.
			orig := ctx
			ctx = withContext(ctx)
			ctx = logger.WithLoggerContext(ctx)

			incoming := r.Header.Get(header)
			if !isValidLogID(incoming) {
				incoming = ""
			}
			if opt.TrustIncoming && incoming != "" {
				SetLogID(ctx, incoming)
			}
			logID := EnsureLogID(ctx)
			logger.AddInfo(ctx, "logid", logID)
			if incoming != "" && incoming != logID {
				logger.AddInfo(ctx, "upstream_request_id", incoming)
			}
			if !opt.OmitResponseHeader {
				w.Header().Set(header, logID)
			}
			if ctx != orig {
				r = r.WithContext(ctx) // copy the request only if the context changed
			}
			return next(ctx, w, r)
		}
	}
}

// isValidLogID reports whether s is safe to reuse as a log ID.
func isValidLogID(s string) bool {
	if len(s) == 0 || len(s) > maxLogIDLength {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.', c == '_', c == ':', c == '-':
		default:
			return false
		}
	}
	return true
}
