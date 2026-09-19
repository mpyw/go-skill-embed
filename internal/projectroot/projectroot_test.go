package projectroot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The walk decides where a project install lands. Without it, running from a
// subdirectory digs a second skills directory there.
func TestFind(t *testing.T) {
	markers := []string{".agents", ".claude"}

	repo := t.TempDir()
	mkdir(t, filepath.Join(repo, ".git"))
	deep := filepath.Join(repo, "cmd", "tool")
	mkdir(t, deep)

	t.Run("walks up to the repository root", func(t *testing.T) {
		got, err := Find(deep, markers)
		if err != nil {
			t.Fatal(err)
		}
		if resolve(t, got) != resolve(t, repo) {
			t.Errorf("root = %s, want %s", got, repo)
		}
	})

	t.Run("an agent directory nearer than the root wins", func(t *testing.T) {
		nested := filepath.Join(repo, "cmd")
		mkdir(t, filepath.Join(nested, ".claude"))
		t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(nested, ".claude")) })

		got, err := Find(deep, markers)
		if err != nil {
			t.Fatal(err)
		}
		if resolve(t, got) != resolve(t, nested) {
			t.Errorf("root = %s, want %s", got, nested)
		}
	})

	t.Run("outside a repository the search is one step", func(t *testing.T) {
		loose := t.TempDir()
		sub := filepath.Join(loose, "sub")
		mkdir(t, sub)

		got, err := Find(sub, markers)
		if err != nil {
			t.Fatal(err)
		}
		if resolve(t, got) != resolve(t, sub) {
			t.Errorf("root = %s, want %s", got, sub)
		}
	})

	// The home directory holds an agent directory for every agent its owner
	// uses. Landing there turns a project install into a user-wide one.
	t.Run("the home directory is refused", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		mkdir(t, filepath.Join(home, ".claude"))

		if _, err := Find(home, markers); !errors.Is(err, ErrIsHome) {
			t.Errorf("err = %v, want ErrIsHome", err)
		}
	})

	// A repository whose root is the home directory reaches the same answer.
	t.Run("a repository at home is refused too", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		mkdir(t, filepath.Join(home, ".git"))
		under := filepath.Join(home, "notes")
		mkdir(t, under)

		if _, err := Find(under, markers); !errors.Is(err, ErrIsHome) {
			t.Errorf("err = %v, want ErrIsHome", err)
		}
	})
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func resolve(t *testing.T, dir string) string {
	t.Helper()
	out, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// The bound is why the walk stops at the repository root. A marker above the
// root belongs to something else, most often the home directory, and an
// unbounded walk would resolve a project install there.
func TestFindStopsAtTheRepositoryRoot(t *testing.T) {
	outer := t.TempDir()
	mkdir(t, filepath.Join(outer, ".claude"))

	repo := filepath.Join(outer, "repo")
	mkdir(t, filepath.Join(repo, ".git"))
	sub := filepath.Join(repo, "sub")
	mkdir(t, sub)

	got, err := Find(sub, []string{".agents", ".claude"})
	if err != nil {
		t.Fatal(err)
	}
	if resolve(t, got) != resolve(t, repo) {
		t.Errorf("root = %s, want %s; the walk passed the repository root", got, repo)
	}
}
