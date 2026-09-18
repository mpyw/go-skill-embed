package skillembed

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// projectRootOf is what project scope resolves against. WithProjectRoot wins,
// and without it the root is searched for.
//
//declscope:package // install.go asks for the root before asking an agent for its directory
func (in *Installer) projectRootOf(scope Scope) (string, error) {
	if scope != ScopeProject || in.projectRoot != "" {
		return in.projectRoot, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// Derived from the agent table rather than written out, so a seventh agent
	// is searched for without touching this. Five of the six share .agents, so
	// the list is deduplicated before it becomes one os.Stat per entry.
	var markers []string
	seen := map[string]bool{}
	for _, a := range in.agents {
		marker := filepath.Dir(filepath.FromSlash(a.ProjectDir))
		if !seen[marker] {
			seen[marker] = true
			markers = append(markers, marker)
		}
	}
	return projectRootFor(wd, markers)
}

// ErrProjectIsHome reports that project scope resolved to the home directory.
//
// The user scope directories live there. A project installation written into
// them would put one project's skills in front of every other project, and
// `--scope user` already writes there on purpose.
var ErrProjectIsHome = errors.New("skillembed: project scope resolved to the home directory")

// projectRootFor chooses the directory project scope resolves against.
//
// The search starts at dir and stops at the repository root. The first
// directory that already holds one of the markers wins. Without one, the
// repository root does. Outside a repository the search is one step, because
// there is no bound to walk within.
//
// The bound matters. A home directory holds an agent directory for every agent
// its owner uses, so an unbounded walk from anywhere under it reaches one and
// turns a project installation into a user-wide one.
func projectRootFor(dir string, markers []string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	bound := projectRepoRoot(abs)
	if bound == "" {
		bound = abs
	}

	chosen := bound
	for at := abs; ; at = filepath.Dir(at) {
		if projectHasMarker(at, markers) {
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

	if home, err := os.UserHomeDir(); err == nil && projectSameDir(chosen, home) {
		return "", fmt.Errorf("%w, so pass --scope user to write there on purpose"+
			" or --dir to name another directory", ErrProjectIsHome)
	}
	return chosen, nil
}

// projectRepoRoot is the nearest ancestor of dir holding a .git, or "" when
// dir is not in a repository.
func projectRepoRoot(dir string) string {
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

// projectHasMarker reports whether dir already holds an agent directory.
func projectHasMarker(dir string, markers []string) bool {
	for _, m := range markers {
		if info, err := os.Stat(filepath.Join(dir, m)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// projectSameDir compares two paths with symbolic links resolved, since a home
// directory reached through one is still the home directory.
func projectSameDir(a, b string) bool {
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
