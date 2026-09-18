// Package projectroot finds the directory a project scope install writes into.
//
// Nothing here knows about skills. It walks a file system and answers with a
// directory.
package projectroot

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrIsHome reports that the search landed on the home directory.
var ErrIsHome = errors.New("the search landed on the home directory")

// Find chooses the root, starting at dir and walking up.
//
// The walk stops at the repository root. The first directory that already
// holds one of the markers wins. Without one, the repository root does.
// Outside a repository the walk is one step, because there is no bound to walk
// within.
//
// The bound matters. A home directory holds a marker for every agent its owner
// uses, so an unbounded walk from anywhere under it reaches one and turns a
// project installation into a user-wide one. Landing on the home directory
// itself is ErrIsHome for the same reason.
func Find(dir string, markers []string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	bound := repoRoot(abs)
	if bound == "" {
		bound = abs
	}

	chosen := bound
	for at := abs; ; at = filepath.Dir(at) {
		if hasMarker(at, markers) {
			chosen = at
			break
		}
		if at == bound {
			break
		}
		if parent := filepath.Dir(at); parent == at {
			break
		}
	}

	if home, err := os.UserHomeDir(); err == nil && sameDir(chosen, home) {
		return "", ErrIsHome
	}
	return chosen, nil
}

// repoRoot is the nearest ancestor of dir holding a .git, or "" when dir is
// not in a repository. A worktree and a submodule have a .git file rather than
// a directory, so both count.
func repoRoot(dir string) string {
	for at := dir; ; at = filepath.Dir(at) {
		if _, err := os.Stat(filepath.Join(at, ".git")); err == nil {
			return at
		}
		parent := filepath.Dir(at)
		if parent == at {
			return ""
		}
	}
}

// hasMarker reports whether dir holds one of the markers as a directory.
func hasMarker(dir string, markers []string) bool {
	for _, m := range markers {
		if info, err := os.Stat(filepath.Join(dir, m)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// sameDir compares two paths with symbolic links resolved, since a home
// directory reached through one is still the home directory.
func sameDir(a, b string) bool {
	if a == b {
		return true
	}
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false
	}
	return ra == rb
}
