package main

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVendor(t *testing.T) {
	for _, scenario := range []string{"pretty JSON", "invalid version", "invalid JSON", "resolution failure", "bundle failure", "license failure"} {
		t.Run(scenario, func(t *testing.T) {
			bundle := []byte(`var mermaid = {}; // path coordinates 11.041.208`)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/packages/npm/mermaid/resolved":
					if r.URL.Query().Get("specifier") != "11" {
						t.Errorf("unexpected specifier: %s", r.URL.RawQuery)
					}
					if scenario == "resolution failure" {
						http.Error(w, "unavailable", 503)
						return
					}
					if scenario == "invalid JSON" {
						fmt.Fprint(w, "not JSON")
						return
					}
					if scenario == "invalid version" {
						fmt.Fprint(w, `{"version":"11.041.208"}`)
						return
					}
					fmt.Fprint(w, "{\n  \"version\": \"11.17.2\"\n}\n")
				case "/npm/mermaid@11.17.2/dist/mermaid.min.js":
					if scenario == "bundle failure" {
						http.Error(w, "unavailable", 503)
						return
					}
					w.Write(bundle)
				case "/npm/mermaid@11.17.2/LICENSE":
					if scenario == "license failure" {
						http.Error(w, "unavailable", 503)
						return
					}
					fmt.Fprint(w, "MIT license")
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			dest := t.TempDir()
			for _, name := range []string{"mermaid.min.js", "mermaid.LICENSE", "mermaid.version"} {
				if err := os.WriteFile(filepath.Join(dest, name), []byte("previous"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			err := vendor(srv.Client(), "11", dest, srv.URL, srv.URL)
			if scenario != "pretty JSON" {
				if err == nil {
					t.Fatal("expected failure")
				}
				for _, name := range []string{"mermaid.min.js", "mermaid.LICENSE", "mermaid.version"} {
					data, err := os.ReadFile(filepath.Join(dest, name))
					if err != nil {
						t.Fatal(err)
					}
					if string(data) != "previous" {
						t.Errorf("%s changed after failed download", name)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			metadata, err := os.ReadFile(filepath.Join(dest, "mermaid.version"))
			if err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("mermaid 11.17.2\nsha256 %x\nsource %s/npm/mermaid@11.17.2/dist/mermaid.min.js\n", sha256.Sum256(bundle), srv.URL)
			if string(metadata) != want {
				t.Errorf("metadata: %s", metadata)
			}
			got, err := os.ReadFile(filepath.Join(dest, "mermaid.min.js"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(bundle) {
				t.Fatal("bundle differs")
			}
			license, err := os.ReadFile(filepath.Join(dest, "mermaid.LICENSE"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(license), "MIT") {
				t.Fatal("license differs")
			}
		})
	}
}
