package skillembed

import (
	"errors"
	"fmt"
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

// ErrProjectEscapes reports that a project scope destination is taken outside
// the project root by a symbolic link.
//
// The path a project install writes to comes from the project, so a link
// committed at .claude/skills, or at any directory above it, aims the write
// and a later forced removal wherever it points. Cloning a repository and
// running the tool once is the whole of it. User scope and --dir are the user
// naming a place, and are not bounded this way.
var ErrProjectEscapes = projectroot.ErrOutsideRoot

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
	root, err := projectroot.Find(wd, markers)
	if err != nil {
		if errors.Is(err, ErrProjectIsHome) {
			// The search says what happened and where. What to do about it is
			// in this tool's vocabulary, which is not projectroot's to know.
			return "", fmt.Errorf("skillembed: %w, so pass --scope user to write"+
				" there on purpose or --dir to name another directory", err)
		}
		return "", err
	}
	return root, nil
}
