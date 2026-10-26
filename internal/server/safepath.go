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
	if strings.ContainsRune(rel, '\x00') {
		return "", false
	}
	// 1) Normalize and join under root. "../" is clamped to the root.
	clean := filepath.Clean("/" + filepath.ToSlash(rel))
	abs := filepath.Join(s.root, clean)

	// 2) Confirm the joined path is root-relative.
	if local, err := filepath.Rel(s.root, abs); err != nil || !filepath.IsLocal(local) {
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
	resolvedRel, err := filepath.Rel(s.root, real)
	if err != nil || !filepath.IsLocal(resolvedRel) {
		return "", false
	}
	suffix, err := filepath.Rel(probe, abs)
	if err != nil {
		return "", false
	}
	return filepath.Join(real, suffix), true
}

// openFile opens a previously resolved path beneath a directory handle. Each
// component must still be the same non-symlink object after opening; this also
// rejects replacements that point to hidden files inside the root.
func (s *Server) openFile(abs string) (*os.File, error) {
	rel, err := filepath.Rel(s.root, abs)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, os.ErrPermission
	}
	root, err := s.openRoot()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, part := range parts[:len(parts)-1] {
		before, err := root.Lstat(part)
		if err != nil {
			return nil, err
		}
		if !before.IsDir() {
			return nil, os.ErrPermission
		}
		next, err := root.OpenRoot(part)
		if err != nil {
			return nil, err
		}
		after, err := next.Stat(".")
		if err != nil || !os.SameFile(before, after) {
			_ = next.Close()
			return nil, os.ErrPermission
		}
		_ = root.Close()
		root = next
	}
	name := parts[len(parts)-1]
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, os.ErrPermission
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		_ = f.Close()
		return nil, os.ErrPermission
	}
	return f, nil
}

// openRoot rejects replacement of the serving directory itself since startup.
func (s *Server) openRoot() (*os.Root, error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	info, err := root.Stat(".")
	if err != nil || s.rootInfo == nil || !os.SameFile(s.rootInfo, info) {
		_ = root.Close()
		return nil, os.ErrPermission
	}
	return root, nil
}
