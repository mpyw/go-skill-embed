package skillfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mpyw/go-skill-embed/internal/manifest"
)

func demoFS() fstest.MapFS {
	return fstest.MapFS{
		manifest.FileName:   &fstest.MapFile{Data: []byte("---\nname: demo\n---\n\n# Demo\n")},
		"scripts/run.sh":    &fstest.MapFile{Data: []byte("#!/bin/sh\necho hi\n")},
		"reference/tips.md": &fstest.MapFile{Data: []byte("tips\n")},
	}
}

func TestDigestIgnoresInjectedMetadata(t *testing.T) {
	plain := demoFS()

	stamped := demoFS()
	stamped[manifest.FileName] = &fstest.MapFile{
		Data: manifest.With(plain[manifest.FileName].Data, []manifest.Entry{
			{Key: manifest.KeyEmbeddedBy, Value: "mytool"},
		}),
	}

	a, err := Digest(plain)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Digest(stamped)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("digest changed when metadata was injected:\n%s\n%s", a, b)
	}
}

func TestDigestNoticesAnEdit(t *testing.T) {
	before, err := Digest(demoFS())
	if err != nil {
		t.Fatal(err)
	}
	edited := demoFS()
	edited["reference/tips.md"] = &fstest.MapFile{Data: []byte("tips, edited\n")}
	after, err := Digest(edited)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Error("digest did not change after an edit")
	}
}

func TestWriteRestoresTheExecutableBit(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo")
	if err := Write(t.Context(), demoFS(), dest, WriteOptions{}); err != nil {
		t.Fatal(err)
	}

	script, err := os.Stat(filepath.Join(dest, "scripts", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	// Where the platform carries no executable bit there is none to set, and
	// the mode os.Stat reports is the read-only attribute rather than anything
	// Write chose.
	if modesMatter && script.Mode().Perm()&0o111 == 0 {
		t.Errorf("scripts/run.sh mode = %v, want the executable bit set", script.Mode().Perm())
	}

	doc, err := os.Stat(filepath.Join(dest, "reference", "tips.md"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Mode().Perm()&0o111 != 0 {
		t.Errorf("reference/tips.md mode = %v, want no executable bit", doc.Mode().Perm())
	}
}

func TestWriteReplacesWhatIsThere(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dest, "gone.md")
	if err := os.WriteFile(stale, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Write(t.Context(), demoFS(), dest, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("a file from the previous installation survived")
	}
}

func TestWriteTransforms(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo")
	err := Write(t.Context(), demoFS(), dest, WriteOptions{
		Transform: func(name string, data []byte) ([]byte, error) {
			if name != manifest.FileName {
				return data, nil
			}
			return append(data, []byte("appended\n")...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if want := "appended\n"; len(got) < len(want) || string(got[len(got)-len(want):]) != want {
		t.Errorf("Transform not applied:\n%s", got)
	}
}

// A file browser leaves a .DS_Store beside an installed skill. That is not the
// user editing the skill, so the digest must not move.
func TestDigestIgnoresOperatingSystemJunk(t *testing.T) {
	before, err := Digest(demoFS())
	if err != nil {
		t.Fatal(err)
	}

	littered := demoFS()
	littered[".DS_Store"] = &fstest.MapFile{Data: []byte("\x00\x01binary")}
	littered["reference/.DS_Store"] = &fstest.MapFile{Data: []byte("\x00\x01binary")}

	after, err := Digest(littered)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("a .DS_Store moved the digest:\n%s\n%s", before, after)
	}
}

func TestWriteSkipsOperatingSystemJunk(t *testing.T) {
	src := demoFS()
	src[".DS_Store"] = &fstest.MapFile{Data: []byte("\x00\x01binary")}

	dest := filepath.Join(t.TempDir(), "demo")
	if err := Write(t.Context(), src, dest, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".DS_Store")); err == nil {
		t.Error(".DS_Store was installed")
	}
}

// The swap is two renames with the old directory moved aside. A failure has to
// leave the existing installation exactly as it was.
func TestWriteLeavesTheOldTreeOnFailure(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo")
	if err := Write(t.Context(), demoFS(), dest, WriteOptions{}); err != nil {
		t.Fatal(err)
	}

	err := Write(t.Context(), demoFS(), dest, WriteOptions{
		Transform: func(name string, data []byte) ([]byte, error) {
			if name == "reference/tips.md" {
				return nil, errors.New("no")
			}
			return data, nil
		},
	})
	if err == nil {
		t.Fatal("the failing write reported success")
	}

	for _, p := range []string{manifest.FileName, "scripts/run.sh", "reference/tips.md"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(p))); err != nil {
			t.Errorf("%s did not survive the failed write: %v", p, err)
		}
	}

	// Nothing is left beside it either.
	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".demo.") {
			t.Errorf("a staging or aside directory was left behind: %s", e.Name())
		}
	}
}

// safeJoin is the last thing between a path inside an embedded skill and a
// write outside the destination. embed.FS cannot produce one of these, but the
// source is an fs.FS and anything may implement that, so the check is the
// guarantee rather than the file system.
func TestSafeJoinRejectsAnEscape(t *testing.T) {
	root := t.TempDir()

	for _, p := range []string{"..", "../evil", "../../evil", "a/../../evil", "/etc/passwd"} {
		if got, err := safeJoin(root, p); err == nil {
			t.Errorf("safeJoin(%q) = %q, want an error", p, got)
		}
	}
	for _, c := range []struct{ in, want string }{
		{".", root},
		{"a.md", filepath.Join(root, "a.md")},
		{"a/b/c.md", filepath.Join(root, "a", "b", "c.md")},
		{"./a/../b.md", filepath.Join(root, "b.md")},
	} {
		got, err := safeJoin(root, c.in)
		if err != nil {
			t.Errorf("safeJoin(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("safeJoin(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Write has to consult it rather than trust the walk. A file system that names
// a file "../evil" must not put one next to the destination.
func TestWriteRefusesAnEscapingPath(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "demo")

	err := Write(t.Context(), escapingFS{demoFS()}, dest, WriteOptions{})
	if err == nil {
		t.Fatal("a file named ../evil was written")
	}
	if !strings.Contains(err.Error(), "unsafe path") {
		t.Errorf("err = %v, want it to name the unsafe path", err)
	}
	if _, statErr := os.Stat(filepath.Join(parent, "evil")); statErr == nil {
		t.Error("the file landed outside the destination")
	}
	// Nothing is left half written either.
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("the destination was created by a refused write")
	}
}

// escapingFS is a file system whose root directory names an entry that climbs
// out of it. No real one does, which is the point: Write cannot assume it.
type escapingFS struct{ fstest.MapFS }

func (escapingFS) ReadDir(string) ([]fs.DirEntry, error) {
	return []fs.DirEntry{escapingEntry{}}, nil
}

type escapingEntry struct{}

func (escapingEntry) Name() string               { return "../evil" }
func (escapingEntry) IsDir() bool                { return false }
func (escapingEntry) Type() fs.FileMode          { return 0 }
func (escapingEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrNotExist }

// The digest cannot see a mode, because an embedded file carries none. This is
// the only check that notices a script which lost the bit it needs to run, and
// the only one that would notice a file handed one it should not have.
func TestExecutableBitsMatch(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo")
	if err := Write(t.Context(), demoFS(), dest, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	installed := os.DirFS(dest)

	match := func(t *testing.T, rule func(string, []byte) bool) bool {
		t.Helper()
		ok, err := ExecutableBitsMatch(installed, rule)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	if !match(t, HasShebang) {
		t.Error("a freshly written tree does not match the rule that wrote it")
	}
	// No rule is a caller that does not care, not a caller that disagrees.
	if !match(t, nil) {
		t.Error("a nil rule reported a mismatch")
	}

	// What follows is a mode on the disk disagreeing with the rule, which is a
	// thing only a platform carrying the bit can arrange. On the other one the
	// two assertions above are the whole of the contract, and
	// TestExecutableBitsMatchWithoutModes states the rest of it.
	if modesMatter {
		script := filepath.Join(dest, "scripts", "run.sh")
		if err := os.Chmod(script, 0o644); err != nil {
			t.Fatal(err)
		}
		if match(t, HasShebang) {
			t.Error("a script that lost its executable bit went unnoticed")
		}
		if err := os.Chmod(script, 0o755); err != nil {
			t.Fatal(err)
		}

		// A bit the rule never asked for is a mismatch in the other direction.
		if match(t, func(name string, _ []byte) bool { return name == "reference/tips.md" }) {
			t.Error("a file carrying a bit the rule does not want went unnoticed")
		}
	}

	// Junk is skipped here as everywhere else, so an executable .DS_Store
	// dropped beside the skill does not make it look broken.
	if err := os.WriteFile(filepath.Join(dest, ".DS_Store"), []byte("\x00\x01binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !match(t, HasShebang) {
		t.Error("an executable .DS_Store was read as part of the skill")
	}
}

// Once dest holds the new tree the write has succeeded. Removing the directory
// that was moved aside is cleanup, and a failure there is not a failed install.
func TestWriteSucceedsWhenTheAsideTreeCannotBeRemoved(t *testing.T) {
	// The directory is made unremovable with a mode. On Windows os.Chmod sets
	// the read-only attribute, which does not stop a child being unlinked and
	// which os.RemoveAll clears on its own, so the path this test is for is
	// never entered and it would pass without reaching anything.
	if runtime.GOOS == "windows" {
		t.Skip("a mode cannot make a directory unremovable here")
	}
	root := t.TempDir()
	// The locked directory is renamed aside during the write, so it has to be
	// found again by walking rather than by the name it started under.
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				_ = os.Chmod(p, 0o755)
			}
			return nil
		})
	})

	dest := filepath.Join(root, "demo")
	if err := Write(t.Context(), demoFS(), dest, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	// A subdirectory nothing may unlink from.
	locked := filepath.Join(dest, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}

	if err := Write(t.Context(), demoFS(), dest, WriteOptions{}); err != nil {
		t.Fatalf("a cleanup failure was reported as a write failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, manifest.FileName)); err != nil {
		t.Errorf("the new tree is not in place: %v", err)
	}
}

// Where the platform has no executable bit, every tree matches whatever the
// rule says and the digest decides alone. Without that, a skill holding a
// shebang file could never read as up-to-date on Windows: list said outdated
// on every run and install rewrote it every time.
func TestExecutableBitsMatchWithoutModes(t *testing.T) {
	if modesMatter {
		t.Skip("the platform carries an executable bit, so the rule is compared against it")
	}
	dest := filepath.Join(t.TempDir(), "demo")
	if err := Write(t.Context(), demoFS(), dest, WriteOptions{}); err != nil {
		t.Fatal(err)
	}

	// A rule that disagrees with the tree in both directions at once. On a
	// platform carrying the bit this is two mismatches.
	upsideDown := func(name string, data []byte) bool { return !HasShebang(name, data) }
	for _, rule := range []func(string, []byte) bool{nil, HasShebang, upsideDown} {
		if ok, err := ExecutableBitsMatch(os.DirFS(dest), rule); err != nil || !ok {
			t.Errorf("ExecutableBitsMatch = %v, %v; want true, nil", ok, err)
		}
	}
}

// Write falls back to the shebang rule when no rule is given, so the check that
// decides whether an installed copy still matches has to fall back to it too.
func TestExecutableBitsMatchFallsBackTheSameWay(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo")
	if err := Write(t.Context(), demoFS(), dest, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if ok, err := ExecutableBitsMatch(os.DirFS(dest), nil); err != nil || !ok {
		t.Fatalf("a fresh install does not match: ok=%v err=%v", ok, err)
	}
	if !modesMatter {
		return
	}
	if err := os.Chmod(filepath.Join(dest, "scripts", "run.sh"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, err := ExecutableBitsMatch(os.DirFS(dest), nil); err != nil || ok {
		t.Errorf("a lost executable bit went unnoticed: ok=%v err=%v", ok, err)
	}
}

// When the swap fails and the destination cannot be restored either, the tree
// moved aside is the only copy left. Reaping it there is the whole loss, so it
// is kept and the error names it.
//
// The window is reached by racing something that recreates the destination
// between the two renames. The test skips when the race does not land, and
// fails when the old tree is gone.
func TestWriteKeepsTheOldTreeWhenTheRestoreFails(t *testing.T) {
	const marker = "OLD-MARKER"

	raced := false
	for range 400 {
		parent := t.TempDir()
		dest := filepath.Join(parent, "demo")
		if err := Write(t.Context(), demoFS(), dest, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, marker), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}

		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				select {
				case <-stop:
					return
				default:
					_ = os.MkdirAll(filepath.Join(dest, "junk"), 0o755)
				}
			}
		}()

		err := Write(t.Context(), demoFS(), dest, WriteOptions{})
		close(stop)
		<-done

		if err == nil {
			continue
		}
		raced = true
		if !survives(t, parent, marker) {
			t.Fatalf("the old tree was removed after a failed write: %v", err)
		}
	}
	if !raced {
		t.Skip("the race never landed, so nothing was proved")
	}
}

// survives reports whether name is anywhere under dir.
func survives(t *testing.T, dir, name string) bool {
	t.Helper()
	found := false
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == name {
			found = true
		}
		return nil
	})
	return found
}
