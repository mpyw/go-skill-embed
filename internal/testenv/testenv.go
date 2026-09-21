// Package testenv holds what a test has to do differently from one machine to
// the next, so that one test body says the same thing everywhere.
package testenv

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// SetHome points every user scope directory at dir for the duration of the
// test.
//
// os.UserHomeDir reads HOME everywhere except Windows, where it reads
// USERPROFILE. A test that sets one of them passes on the platform that reads
// it and reaches the developer's real home directory on the other, which is
// where a user scope install would then land.
//
// CLAUDE_CONFIG_DIR is cleared for the same reason. It overrides the
// home-derived path outright, so a developer who has it set ran a suite that
// resolved user scope somewhere this function had not pointed.
func SetHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
}

var (
	symlinkOnce sync.Once
	symlinkErr  error
)

// RequireSymlink skips the test when this process may not create a symbolic
// link, and fails it on CI.
//
// Windows gates them behind a privilege an account may or may not hold, so the
// platform does not settle it and making one is the only way to learn the
// answer. A developer without the privilege should still be able to run the
// suite, which is why it skips.
//
// CI is the other case. `go test` prints a skip only under -v, so a runner
// image that stopped granting the privilege would drop every symbolic link
// test, and the whole of projectroot.Within with them, behind an "ok" line
// that reads exactly like one that ran them.
func RequireSymlink(t *testing.T) {
	t.Helper()
	symlinkOnce.Do(func() {
		dir, err := os.MkdirTemp("", "symlink-probe")
		if err != nil {
			symlinkErr = err
			return
		}
		defer func() { _ = os.RemoveAll(dir) }()
		symlinkErr = os.Symlink(filepath.Join(dir, "target"), filepath.Join(dir, "link"))
	})
	if symlinkErr == nil {
		return
	}
	if os.Getenv("GITHUB_ACTIONS") != "" {
		t.Fatalf("symbolic links are not available on this runner, so this test would be skipped in silence: %v", symlinkErr)
	}
	t.Skipf("symbolic links are not available here: %v", symlinkErr)
}
