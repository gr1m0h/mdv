package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHostBoundary(t *testing.T) {
	s, _ := newTestServer(t)
	for _, path := range []string{"/", "/README.md", "/__mdv/fragment?path=README.md", "/__mdv/events?path=README.md", "/__mdv/custom.css", "/__mdv/assets/mdv.js", "/image.png"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Host = "attacker.example:4649"
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Fatalf("%s: got %d, want 403", path, rec.Code)
		}
	}
	for _, host := range []string{"localhost:4649", "LOCALHOST:4649", "localhost.", "127.0.0.1:4649", "[::1]:4649", "[::1]"} {
		req := httptest.NewRequest("GET", "/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Errorf("%s: got %d", host, rec.Code)
		}
	}
}

func TestConfiguredHostOverHTTP(t *testing.T) {
	for _, tc := range []struct {
		bind, host string
		want       int
	}{
		{"", "attacker.example:4649", 403},
		{"", "localhost.attacker.example", 403},
		{"", "localhost@attacker.example", 403},
		{"", "localhost:bad", 403},
		{"", "192.168.1.2:4649", 403},
		{"notes.local", "NOTES.LOCAL.:4649", 200},
		{"notes.local", "attacker.example:4649", 403},
		{"192.168.1.2", "192.168.1.2:4649", 200},
		{"192.168.1.2", "192.168.1.3:4649", 403},
		{"0.0.0.0", "192.168.1.2:4649", 200},
		{"0.0.0.0", "attacker.example:4649", 403},
		{"::", "[2001:db8::1]:4649", 200},
		{"::", "attacker.example:4649", 403},
	} {
		t.Run(tc.bind+"/"+tc.host, func(t *testing.T) {
			s, _ := newTestServer(t)
			s = New(Config{Root: s.root, BindHost: tc.bind, Quiet: true})
			ts := httptest.NewServer(s)
			defer ts.Close()
			req, err := http.NewRequest("GET", ts.URL+"/README.md", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Host = tc.host
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("got %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

// Exercise the gap between validation and opening, without relying on timing.
