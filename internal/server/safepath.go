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
	if hiddenPath(rel) {
		return "", false
	}
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
	resolvedRel, err := filepath.Rel(s.root, real)
	if err != nil || hiddenPath(resolvedRel) {
		return "", false
	}
	return abs, true
}

// hiddenPath ignores navigation components, but rejects dotfiles and dotdirs.
func hiddenPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
	}
	return false
}
