package projectroot

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mpyw/go-skill-embed/internal/testenv"
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
		testenv.SetHome(t, home)
		mkdir(t, filepath.Join(home, ".claude"))

		if _, err := Find(home, markers); !errors.Is(err, ErrIsHome) {
			t.Errorf("err = %v, want ErrIsHome", err)
		}
	})

	// A repository whose root is the home directory reaches the same answer.
	t.Run("a repository at home is refused too", func(t *testing.T) {
		home := t.TempDir()
		testenv.SetHome(t, home)
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

func TestWithin(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	mkdir(t, filepath.Join(root, ".claude"))

	t.Run("a directory that is not there yet", func(t *testing.T) {
		// The usual case. Nothing below .claude exists, and the answer still
		// has to be where the directories would be created.
		if err := Within(root, filepath.Join(root, ".claude", "skills")); err != nil {
			t.Errorf("Within = %v, want nil", err)
		}
	})

	t.Run("a link that stays inside", func(t *testing.T) {
		mkdir(t, filepath.Join(root, "shared"))
		link := filepath.Join(root, ".claude", "inside")
		symlink(t, filepath.Join(root, "shared"), link)

		if err := Within(root, filepath.Join(link, "skills")); err != nil {
			t.Errorf("Within = %v, want nil; a link within the project is the project's own", err)
		}
	})

	t.Run("a link that leaves", func(t *testing.T) {
		link := filepath.Join(root, ".claude", "skills")
		symlink(t, outside, link)

		err := Within(root, filepath.Join(link, "demo"))
		if !errors.Is(err, ErrOutsideRoot) {
			t.Fatalf("Within = %v, want ErrOutsideRoot", err)
		}
		// Naming the real destination is the whole point of the message. A
		// reader who is told only that something is wrong cannot find the link.
		if got := err.Error(); !strings.Contains(got, resolve(t, outside)) {
			t.Errorf("error %q does not name where the link leads", got)
		}
	})

	t.Run("the root itself reached through a link", func(t *testing.T) {
		// macOS hands out /var and /tmp as links, so a temporary directory is
		// one. Comparing unresolved paths would reject every project there.
		alias := filepath.Join(t.TempDir(), "alias")
		symlink(t, root, alias)

		if err := Within(alias, filepath.Join(alias, ".claude", "skills")); err != nil {
			t.Errorf("Within = %v, want nil", err)
		}
	})

	t.Run("a link whose target is not there yet", func(t *testing.T) {
		// EvalSymlinks reports a dangling link as missing, which is what an
		// ordinary directory about to be created looks like. Read as that, the
		// link would pass as a path inside the project.
		gone := filepath.Join(outside, "not-yet")
		link := filepath.Join(root, ".claude", "dangling")
		symlink(t, gone, link)

		if err := Within(root, filepath.Join(link, "demo")); !errors.Is(err, ErrOutsideRoot) {
			t.Errorf("Within = %v, want ErrOutsideRoot", err)
		}
	})

	t.Run("a sibling whose name starts with the root", func(t *testing.T) {
		// A string prefix test without the separator reads /a/project-notes as
		// being inside /a/project.
		if err := Within(root, root+"-notes"); !errors.Is(err, ErrOutsideRoot) {
			t.Errorf("Within = %v, want ErrOutsideRoot", err)
		}
	})

	t.Run("a relative root", func(t *testing.T) {
		// WithProjectRoot takes what it is given, and Targets resolves the
		// destination with filepath.Abs before the check. Comparing one
		// against the other refuses every project install.
		t.Chdir(filepath.Dir(root))

		if err := Within(filepath.Base(root), filepath.Join(root, ".claude", "skills")); err != nil {
			t.Errorf("Within = %v, want nil", err)
		}
	})

	t.Run("the volume root", func(t *testing.T) {
		// A root already ending in a separator, so the prefix to test for
		// would be "//" and would match nothing. Find returns the working
		// directory when there is no repository and no marker, and that can
		// be the volume root.
		volume := filepath.VolumeName(root) + string(filepath.Separator)

		if err := Within(volume, filepath.Join(volume, ".claude", "skills")); err != nil {
			t.Errorf("Within = %v, want nil", err)
		}
	})

	t.Run("a deep path with no link in it", func(t *testing.T) {
		// The hop budget is for links. Spending it on the walk up to the first
		// existing ancestor turns a deep destination into "too many symbolic
		// links".
		deep := root
		for range maxLinkHops * 2 {
			deep = filepath.Join(deep, "d")
		}

		if err := Within(root, deep); err != nil {
			t.Errorf("Within = %v, want nil", err)
		}
	})
}

// filepath.Rel answers with an error when the two paths are on different
// volumes, and Within reads that as outside. Nothing but Windows has a second
// volume, and a Windows machine may have only one, so the test goes looking
// for one rather than naming a letter.
func TestWithinAcrossVolumes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("one volume")
	}
	root := t.TempDir()

	var other string
	for letter := 'A'; letter <= 'Z' && other == ""; letter++ {
		volume := string(letter) + ":"
		if strings.EqualFold(volume, filepath.VolumeName(root)) {
			continue
		}
		if _, err := os.Stat(volume + string(filepath.Separator)); err == nil {
			other = volume + string(filepath.Separator)
		}
	}
	if other == "" {
		t.Skip("this machine has one volume")
	}

	err := Within(root, filepath.Join(other, "skills"))
	if !errors.Is(err, ErrOutsideRoot) {
		t.Errorf("Within = %v, want ErrOutsideRoot", err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	testenv.RequireSymlink(t)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(link) })
}
