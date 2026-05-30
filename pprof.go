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

	wrap := func(h http.HandlerFunc) http.HandlerFunc {
		if !opt.LoopbackOnly {
			return h
		}
		return func(w http.ResponseWriter, r *http.Request) {
			if !isLoopback(r) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			h(w, r)
		}
	}

	prefix := strings.TrimRight(opt.Prefix, "/")
	r.mux.HandleFunc(prefix+"/", wrap(pprof.Index))
	r.mux.HandleFunc(prefix+"/cmdline", wrap(pprof.Cmdline))
	r.mux.HandleFunc(prefix+"/profile", wrap(pprof.Profile))
	r.mux.HandleFunc(prefix+"/symbol", wrap(pprof.Symbol))
	r.mux.HandleFunc(prefix+"/trace", wrap(pprof.Trace))
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
