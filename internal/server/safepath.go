package server

import (
	"os"
	"path/filepath"
	"strings"
)

// safeResolve maps a request-relative path to an absolute path under the root,
// rejecting any attempt to escape via "../" or symlinks. It returns the
// resolved absolute path and true when the path is safe. See spec §8.1.
//
// The symlink check (step 3) is applied to the deepest existing ancestor so
// that a legitimate typo for a non-existent file yields 404 (handled by the
// caller via os.Stat) rather than 403.
func (s *Server) safeResolve(rel string) (string, bool) {
	// 1) Normalize and join under root. "../" is clamped to the root.
	clean := filepath.Clean("/" + filepath.ToSlash(rel))
	abs := filepath.Join(s.root, clean)

	// 2) String prefix check.
	if abs != s.root && !strings.HasPrefix(abs, s.root+string(filepath.Separator)) {
		return "", false
	}

	// 3) EvalSymlinks on the deepest existing ancestor and re-check.
	probe := abs
	for len(probe) > len(s.root) {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		probe = filepath.Dir(probe)
	}
	real, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return "", false
	}
	if real != s.root && !strings.HasPrefix(real, s.root+string(filepath.Separator)) {
		return "", false
	}
	return abs, true
}
