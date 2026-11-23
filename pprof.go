package golitekit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"
)

// PprofOptions configures pprof route mounting.
type PprofOptions struct {
	Prefix string // URL prefix, defaults to "/debug/pprof"
	// LoopbackOnly allows only loopback clients without forwarding headers.
	// Prefer a separate loopback listener (ServePprof / pprofAddr).
	LoopbackOnly bool
}

func (o PprofOptions) prefix() string {
	prefix := o.Prefix
	if prefix == "" {
		prefix = "/debug/pprof"
	}
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		// A root prefix would collide with the router's fallback route.
		panic("golitekit: pprof prefix must not be the root path")
	}
	return prefix
}

// pprofHandlers returns the pprof endpoints keyed by pattern.
func pprofHandlers(opt PprofOptions) map[string]http.Handler {
	wrap := func(h http.HandlerFunc) http.Handler {
		if !opt.LoopbackOnly {
			return h
		}
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !isLoopback(req) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			h(w, req)
		})
	}

	prefix := opt.prefix()
	// pprof.Index only knows the "/debug/pprof/" prefix, so named profiles
	// (heap, goroutine, ...) are dispatched here.
	index := func(w http.ResponseWriter, req *http.Request) {
		if name := strings.TrimPrefix(req.URL.Path, prefix+"/"); name != "" && name != req.URL.Path {
			pprof.Handler(name).ServeHTTP(w, req)
			return
		}
		pprof.Index(w, req)
	}
	return map[string]http.Handler{
		prefix + "/":        wrap(index),
		prefix + "/cmdline": wrap(pprof.Cmdline),
		prefix + "/profile": wrap(pprof.Profile),
		prefix + "/symbol":  wrap(pprof.Symbol),
		prefix + "/trace":   wrap(pprof.Trace),
	}
}

// MountPprof registers pprof handlers on the router's mux. The router's
// timeout middleware also applies, which can cut off long /profile and /trace
// requests; use ServePprof or pprofAddr for those.
func (r *Router) MountPprof(opts ...PprofOptions) {
	var opt PprofOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	r.routesRegistered = true
	for pattern, h := range pprofHandlers(opt) {
		r.mux.Handle(pattern, r.wrapHTTPHandler(h))
	}
}

// ServePprof serves pprof on its own listener at addr until ctx is canceled.
// Bind it to loopback, e.g. "127.0.0.1:6060".
func ServePprof(ctx context.Context, addr string, opts ...PprofOptions) (net.Addr, error) {
	var opt PprofOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	mux := http.NewServeMux()
	for pattern, h := range pprofHandlers(opt) {
		mux.Handle(pattern, h)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("pprof listen: %w", err)
	}
	// No write timeout: /profile and /trace stream for their whole duration.
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "golitekit: pprof server stopped: %v\n", err)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	return ln.Addr(), nil
}

// isLoopback reports whether req comes directly from a loopback client.
func isLoopback(r *http.Request) bool {
	for _, h := range []string{"X-Forwarded-For", "X-Real-IP", "Forwarded"} {
		if r.Header.Get(h) != "" {
			return false // relayed by a proxy: the real client is elsewhere
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
