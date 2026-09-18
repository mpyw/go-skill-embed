package skillembed

import (
	"os"
	"path/filepath"
	"testing"
)

// Naming a directory outright skips the search, and user scope never consults
// it at all.
func TestProjectRootOptionWins(t *testing.T) {
	root := t.TempDir()
	in := NewInstaller(nil, WithProjectRoot(root))

	for _, scope := range []Scope{ScopeProject, ScopeUser} {
		got, err := in.projectRootOf(scope)
		if err != nil {
			t.Fatalf("%s: %v", scope, err)
		}
		if got != root {
			t.Errorf("%s: root = %s, want %s", scope, got, root)
		}
	}
}

// The markers come from the agent table, so restricting the agents restricts
// what the search looks for.
func TestProjectMarkersFollowTheAgents(t *testing.T) {
	root := t.TempDir()
	projectMkdir(t, filepath.Join(root, ".git"))
	sub := filepath.Join(root, "sub")
	projectMkdir(t, filepath.Join(sub, ".agents"))
	t.Chdir(sub)

	// Claude Code alone does not look for .agents, so the search reaches the
	// repository root.
	claude := NewInstaller(nil, WithAgents(AgentClaudeCode))
	got, err := claude.projectRootOf(ScopeProject)
	if err != nil {
		t.Fatal(err)
	}
	if projectResolve(t, got) != projectResolve(t, root) {
		t.Errorf("root = %s, want %s", got, root)
	}

	// Every agent looks for .agents too, so the subdirectory wins.
	all := NewInstaller(nil)
	got, err = all.projectRootOf(ScopeProject)
	if err != nil {
		t.Fatal(err)
	}
	if projectResolve(t, got) != projectResolve(t, sub) {
		t.Errorf("root = %s, want %s", got, sub)
	}
}

func projectMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func projectResolve(t *testing.T, dir string) string {
	t.Helper()
	out, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
