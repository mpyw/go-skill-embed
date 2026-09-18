package skillembed

import (
	"os"
	"path/filepath"

	"github.com/mpyw/go-skill-embed/internal/projectroot"
)

// ErrProjectIsHome reports that project scope resolved to the home directory.
//
// The user scope directories live there. A project installation written into
// them would put one project's skills in front of every other project, and
// --scope user already writes there on purpose.
var ErrProjectIsHome = projectroot.ErrIsHome

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
	return projectroot.Find(wd, markers)
}
