// Command vendor-mermaid resolves and downloads a pinned Mermaid distribution.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var releaseVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func fetch(client *http.Client, address string) ([]byte, error) {
	resp, err := client.Get(address)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", address, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

func vendor(client *http.Client, spec, dest, apiBase, cdnBase string) error {
	data, err := fetch(client, apiBase+"/v1/packages/npm/mermaid/resolved?specifier="+url.QueryEscape(spec))
	if err != nil {
		return err
	}
	var resolved struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &resolved); err != nil {
		return fmt.Errorf("decode resolved version: %w", err)
	}
	if !releaseVersion.MatchString(resolved.Version) {
		return fmt.Errorf("invalid resolved release version %q", resolved.Version)
	}
	base := cdnBase + "/npm/mermaid@" + resolved.Version
	source := base + "/dist/mermaid.min.js"
	bundle, err := fetch(client, source)
	if err != nil {
		return err
	}
	if !bytes.Contains(bundle, []byte("mermaid")) {
		return fmt.Errorf("downloaded bundle does not contain mermaid")
	}
	license, err := fetch(client, base+"/LICENSE")
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(license)) == 0 {
		return fmt.Errorf("downloaded license is empty")
	}
	metadata := fmt.Sprintf("mermaid %s\nsha256 %x\nsource %s\n", resolved.Version, sha256.Sum256(bundle), source)
	// Stage every response before replacing tracked assets, so download failures
	// leave the previous bundle, license and metadata intact.
	staging, err := os.MkdirTemp(dest, ".mermaid-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	files := []struct {
		name string
		data []byte
	}{
		{"mermaid.min.js", bundle}, {"mermaid.LICENSE", license}, {"mermaid.version", []byte(metadata)},
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(staging, f.name), f.data, 0o644); err != nil {
			return err
		}
	}
	for _, f := range files {
		if err := os.Rename(filepath.Join(staging, f.name), filepath.Join(dest, f.name)); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	spec, dest := os.Getenv("MERMAID_SPEC"), os.Getenv("STATIC")
	if spec == "" {
		spec = "11"
	}
	if dest == "" {
		dest = "internal/assets/static"
	}
	if err := vendor(&http.Client{Timeout: time.Minute}, spec, dest, "https://data.jsdelivr.com", "https://cdn.jsdelivr.net"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("updated Mermaid bundle, license and version in", dest)
}
