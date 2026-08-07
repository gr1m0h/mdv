//go:build !windows

package main

import (
	"slices"
	"strings"
	"testing"
)

// Regression: the detached daemon child must inherit the rendering flags the
// user passed, otherwise `mdv -d --theme dark` / `--css x.css` silently fall
// back to auto/no-css because the re-exec dropped them.
func TestDaemonChildArgsForwardsRenderingFlags(t *testing.T) {
	tests := []struct {
		name string
		opts options
		want []string // flags that must be present as "--name value" pairs
	}{
		{
			name: "theme forwarded",
			opts: options{host: "127.0.0.1", theme: "dark", path: "doc.md"},
			want: []string{"--theme", "dark"},
		},
		{
			name: "css forwarded",
			opts: options{host: "127.0.0.1", css: "custom.css", path: "doc.md"},
			want: []string{"--css", "custom.css"},
		},
		{
			name: "theme and css forwarded together",
			opts: options{host: "127.0.0.1", theme: "light", css: "custom.css", path: "doc.md"},
			want: []string{"--theme", "light", "--css", "custom.css"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args := daemonChildArgs(tt.opts, 4649)
			for i := 0; i+1 < len(tt.want); i += 2 {
				flag, val := tt.want[i], tt.want[i+1]
				idx := slices.Index(args, flag)
				if idx < 0 || idx+1 >= len(args) || args[idx+1] != val {
					t.Errorf("daemonChildArgs(%+v) = %v; missing %s %s", tt.opts, args, flag, val)
				}
			}
			if got := args[len(args)-1]; got != tt.opts.path {
				t.Errorf("path must be the final arg; got %q, want %q", got, tt.opts.path)
			}
		})
	}
}

// An empty theme/css (the zero value when the user passes nothing) must NOT
// emit a flag, so the child's own defaulting logic applies.
func TestDaemonChildArgsOmitsEmptyFlags(t *testing.T) {
	t.Parallel()
	args := daemonChildArgs(options{host: "127.0.0.1", path: "doc.md"}, 4649)
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--theme") {
		t.Errorf("empty theme must not emit --theme; got %v", args)
	}
	if strings.Contains(joined, "--css") {
		t.Errorf("empty css must not emit --css; got %v", args)
	}
}
