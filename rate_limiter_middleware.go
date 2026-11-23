package golitekit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// RateLimiterAsMiddleware returns a middleware that enforces rate limits using keyFunc.
func (r *RateLimiter) RateLimiterAsMiddleware(keyFunc func(r *http.Request) string) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
			// The per-key limit is checked first: a client rejected by it must
			// not consume global tokens (Reservation.Cancel does not refund
			// an immediate reservation).
			if keyFunc != nil {
				if !r.limiterForKey(keyFunc(req)).Allow() {
					return ErrTooManyRequests("Rate limit exceeded", nil)
				}
			}

			if r.enableGlobal && r.globalLimiter != nil {
				if !r.globalLimiter.Allow() {
					return ErrTooManyRequests("Global rate limit exceeded", nil)
				}
			}

			return next(ctx, w, req)
		}
	}
}

// ByIP returns the client IP address (without port) for use as a rate limiter key.
// It uses the connection's remote address only; behind a reverse proxy every
// request then shares the proxy's address. Use ByClientIP in that case.
func ByIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ByPath keys by the matched route pattern (e.g. "GET /users/{id}"), never
// the raw path. Unmatched requests share one key.
func ByPath(r *http.Request) string {
	return r.Pattern
}

// ByClientIP keys by the client IP behind trustedProxies (IPs or CIDRs; panics
// if invalid). X-Forwarded-For (right to left) or X-Real-IP is read only when
// the peer is trusted; otherwise the remote address is used.
func ByClientIP(trustedProxies ...string) func(r *http.Request) string {
	trusted := make([]netip.Prefix, 0, len(trustedProxies))
	for _, s := range trustedProxies {
		p, err := parseIPOrPrefix(s)
		if err != nil {
			panic(fmt.Sprintf("golitekit: invalid trusted proxy %q: %v", s, err))
		}
		trusted = append(trusted, p)
	}
	isTrusted := func(ip netip.Addr) bool {
		for _, p := range trusted {
			if p.Contains(ip) {
				return true
			}
		}
		return false
	}

	return func(r *http.Request) string {
		remote := ByIP(r)
		remoteIP, err := netip.ParseAddr(remote)
		if err != nil || !isTrusted(normalizeAddr(remoteIP)) {
			return remote
		}

		if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
			hops := strings.Split(strings.Join(xff, ","), ",")
			client := remote
			for i := len(hops) - 1; i >= 0; i-- {
				ip, ok := parseHopAddr(hops[i])
				if !ok {
					return client // malformed: stop at the last address we trust
				}
				client = ip.String()
				if !isTrusted(ip) {
					return client
				}
			}
			return client // every hop is trusted: the leftmost is the client
		}
		if ip, ok := parseHopAddr(r.Header.Get("X-Real-IP")); ok {
			return ip.String()
		}
		return remote
	}
}

// parseHopAddr parses a forwarding-header address. Some proxies append the
// port ("203.0.113.1:5678", "[2001:db8::1]:443"), which is dropped.
func parseHopAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	ip, err := netip.ParseAddr(s)
	if err != nil {
		ap, err := netip.ParseAddrPort(s)
		if err != nil {
			return netip.Addr{}, false
		}
		ip = ap.Addr()
	}
	return normalizeAddr(ip), true
}

// normalizeAddr unmaps IPv4-mapped IPv6 addresses and drops the zone, which
// netip.Prefix.Contains would otherwise never match.
func normalizeAddr(ip netip.Addr) netip.Addr {
	return ip.Unmap().WithZone("")
}

func parseIPOrPrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, err
		}
		// Addresses are compared unmapped, so "::ffff:10.0.0.0/104" must
		// become "10.0.0.0/8".
		if addr := p.Addr(); addr.Is4In6() && p.Bits() >= 96 {
			p = netip.PrefixFrom(addr.Unmap(), p.Bits()-96)
		}
		return p.Masked(), nil
	}
	ip, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	ip = normalizeAddr(ip)
	return netip.PrefixFrom(ip, ip.BitLen()), nil
}
