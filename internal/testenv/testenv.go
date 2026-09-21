// Package testenv answers the questions a test has to ask about the machine
// it is running on, so that one test body says the same thing everywhere.
package testenv

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// SetHome points the home directory at dir for the duration of the test.
//
// os.UserHomeDir reads HOME everywhere except Windows, where it reads
// USERPROFILE. A test that sets one of them passes on the platform that reads
// it and reaches the developer's real home directory on the other, which is
// where a user scope install would then land.
func SetHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

var (
	symlinkOnce sync.Once
	symlinkErr  error
)

// RequireSymlink skips the test when this process may not create a symbolic
// link.
//
// Windows hands that privilege to an administrator or to a machine in
// developer mode and to nobody else, so the answer belongs to the account
// rather than to the platform and asking is the only way to learn it.
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
	if symlinkErr != nil {
		t.Skipf("symbolic links are not available here: %v", symlinkErr)
	}
}
