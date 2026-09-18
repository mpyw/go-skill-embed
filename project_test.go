package skillembed

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Project scope resolves against the project, not against whichever directory
// the command was run from. Without this, running from a subdirectory digs a
// second skills directory there.
func TestProjectRootSearch(t *testing.T) {
	markers := []string{".agents", ".claude"}

	repo := t.TempDir()
	projectMkdir(t, filepath.Join(repo, ".git"))
	deep := filepath.Join(repo, "cmd", "tool")
	projectMkdir(t, deep)

	t.Run("walks up to the repository root", func(t *testing.T) {
		got, err := projectRootFor(deep, markers)
		if err != nil {
			t.Fatal(err)
		}
		if projectResolve(t, got) != projectResolve(t, repo) {
			t.Errorf("root = %s, want %s", got, repo)
		}
	})

	t.Run("an agent directory nearer than the root wins", func(t *testing.T) {
		nested := filepath.Join(repo, "cmd")
		projectMkdir(t, filepath.Join(nested, ".claude"))
		t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(nested, ".claude")) })

		got, err := projectRootFor(deep, markers)
		if err != nil {
			t.Fatal(err)
		}
		if projectResolve(t, got) != projectResolve(t, nested) {
			t.Errorf("root = %s, want %s", got, nested)
		}
	})

	t.Run("outside a repository the search is one step", func(t *testing.T) {
		loose := t.TempDir()
		sub := filepath.Join(loose, "sub")
		projectMkdir(t, sub)

		got, err := projectRootFor(sub, markers)
		if err != nil {
			t.Fatal(err)
		}
		if projectResolve(t, got) != projectResolve(t, sub) {
			t.Errorf("root = %s, want %s", got, sub)
		}
	})

	// The home directory holds an agent directory for every agent its owner
	// uses. Landing there turns a project install into a user-wide one.
	t.Run("the home directory is refused", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		projectMkdir(t, filepath.Join(home, ".claude"))

		if _, err := projectRootFor(home, markers); !errors.Is(err, ErrProjectIsHome) {
			t.Errorf("err = %v, want ErrProjectIsHome", err)
		}
	})

	// A repository whose root is the home directory reaches the same answer.
	t.Run("a repository at home is refused too", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		projectMkdir(t, filepath.Join(home, ".git"))
		under := filepath.Join(home, "notes")
		projectMkdir(t, under)

		if _, err := projectRootFor(under, markers); !errors.Is(err, ErrProjectIsHome) {
			t.Errorf("err = %v, want ErrProjectIsHome", err)
		}
	})
}

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
