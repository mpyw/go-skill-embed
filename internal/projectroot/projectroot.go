// Package projectroot finds the directory a project scope install writes into.
//
// Nothing here knows about skills. It walks a file system and answers with a
// directory.
package projectroot

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
		return "", fmt.Errorf("%w (%s)", ErrIsHome, chosen)
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

// ErrOutsideRoot reports that a destination leaves the root it was resolved
// against.
var ErrOutsideRoot = errors.New("a symbolic link takes the destination outside the project")

// Within checks that dir stays inside root once symbolic links are resolved.
//
// A path component is followed by every write, so a link at dir or above it
// decides where the bytes land, whatever the path says. Comparing the two
// literal strings cannot see that, and neither can a check on dir alone: the
// link is usually a parent, and usually one the caller never names.
//
// The bound is the reason. A project root is whatever was cloned, so a link
// committed into it is someone else's choice, unlike a home directory moved
// with one.
func Within(root, dir string) error {
	realRoot, err := realPath(root)
	if err != nil {
		return err
	}
	realDir, err := realPath(dir)
	if err != nil {
		return err
	}
	if realDir == realRoot || strings.HasPrefix(realDir, realRoot+string(filepath.Separator)) {
		return nil
	}
	return fmt.Errorf("%w (%s is really %s)", ErrOutsideRoot, dir, realDir)
}

// maxLinkHops bounds the hand resolution below, the way a kernel bounds its
// own. A cycle reaches EvalSymlinks as ELOOP rather than ErrNotExist and is
// returned as the error it is, so this is a backstop and not the guard.
const maxLinkHops = 64

// realPath resolves the symbolic links in p.
//
// filepath.EvalSymlinks needs the whole path to exist, and a skills directory
// usually does not yet. The existing part is resolved and the rest appended,
// which is where the directories about to be created will end up.
func realPath(p string) (string, error) {
	at := filepath.Clean(p)
	var rest []string
	for hop := 0; hop < maxLinkHops; hop++ {
		resolved, err := filepath.EvalSymlinks(at)
		if err == nil {
			return filepath.Join(append([]string{resolved}, rest...)...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		// A link whose target is not there yet still says where a write would
		// go, and EvalSymlinks will not read one: it reports the target as
		// missing, which is indistinguishable from an ordinary missing
		// directory. Reading it by hand keeps a dangling link from passing as
		// one.
		if target, rerr := os.Readlink(at); rerr == nil {
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(at), target)
			}
			at = filepath.Clean(target)
			continue
		}
		parent := filepath.Dir(at)
		if parent == at {
			// The walk reached the volume root without finding anything that
			// exists. Nothing can be resolved, so p stands as it is.
			return filepath.Clean(p), nil
		}
		rest = append([]string{filepath.Base(at)}, rest...)
		at = parent
	}
	return "", fmt.Errorf("%s: too many symbolic links", p)
}
