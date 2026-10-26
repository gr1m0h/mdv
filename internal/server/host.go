package server

import (
	"net"
	"net/netip"
	"strings"
)

// allowedHost compares names without DNS resolution: resolving an untrusted
// hostname to loopback would reintroduce DNS rebinding.
func (s *Server) allowedHost(authority string) bool {
	host := authority
	if h, port, err := net.SplitHostPort(authority); err == nil {
		if port == "" || strings.IndexFunc(port, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return false
		}
		host = h
	} else if strings.HasPrefix(authority, "[") && strings.HasSuffix(authority, "]") {
		host = authority[1 : len(authority)-1]
		if _, err := netip.ParseAddr(host); err != nil {
			return false
		}
	} else if strings.ContainsAny(authority, ":[]") {
		return false
	}
	if host == "" {
		return false
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" {
		return true
	}
	ip, ipErr := netip.ParseAddr(host)
	if ipErr == nil && ip.Unmap().IsLoopback() {
		return true
	}
	bind := strings.TrimSuffix(strings.ToLower(s.bindHost), ".")
	if bind == "" {
		return false
	}
	if boundIP, err := netip.ParseAddr(bind); err == nil {
		return ipErr == nil && (boundIP.IsUnspecified() || ip == boundIP)
	}
	return host == bind
}
