package golitekit

import (
	"net"
	"net/http"
	"net/http/pprof"
	"strings"
)

// PprofOptions configures pprof route mounting.
type PprofOptions struct {
	Prefix       string // URL prefix, defaults to "/debug/pprof"
	LoopbackOnly bool   // restrict to loopback addresses (127.0.0.1, ::1)
}

// MountPprof registers pprof handlers on the router's mux.
func (r *Router) MountPprof(opts ...PprofOptions) {
	opt := PprofOptions{Prefix: "/debug/pprof"}
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.Prefix == "" {
		opt.Prefix = "/debug/pprof"
	}

	wrap := func(h http.HandlerFunc) http.Handler {
		if !opt.LoopbackOnly {
			return r.wrapHTTPHandler(h)
		}
		return r.wrapHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !isLoopback(req) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			h(w, req)
		}))
	}

	r.routesRegistered = true
	prefix := strings.TrimRight(opt.Prefix, "/")
	r.mux.Handle(prefix+"/", wrap(pprof.Index))
	r.mux.Handle(prefix+"/cmdline", wrap(pprof.Cmdline))
	r.mux.Handle(prefix+"/profile", wrap(pprof.Profile))
	r.mux.Handle(prefix+"/symbol", wrap(pprof.Symbol))
	r.mux.Handle(prefix+"/trace", wrap(pprof.Trace))
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
