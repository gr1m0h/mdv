package server

import (
	"strings"
	"testing"
)

func TestResponseSecurityHeaders(t *testing.T) {
	s, _ := newTestServer(t)
	for _, target := range []string{"/", "/README.md", "/image.png", "/__mdv/fragment?path=README.md", "/missing.md"} {
		rec := do(s, "GET", target)
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s lacks nosniff", target)
		}
		if rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s lacks no-referrer", target)
		}
		if target == "/" || target == "/README.md" {
			for _, directive := range []string{"base-uri 'none'", "form-action 'none'", "frame-ancestors 'none'"} {
				if !strings.Contains(rec.Header().Get("Content-Security-Policy"), directive) {
					t.Errorf("%s lacks %s", target, directive)
				}
			}
		}
	}
}
