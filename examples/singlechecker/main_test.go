package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// build produces the real binary. The thing under test is what happens to
// os.Args before singlechecker sees them, which only a separate process shows.
func build(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a binary")
	}
	bin := filepath.Join(t.TempDir(), "examplelint")
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func run(t *testing.T, name string, args ...string) (string, int) {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return string(out), exit.ExitCode()
}

func TestSkillSubcommand(t *testing.T) {
	bin := build(t)
	dest := filepath.Join(t.TempDir(), "skills")

	out, code := run(t, bin, "skill", "install", "--dir", dest)
	if code != 0 {
		t.Fatalf("install exited %d:\n%s", code, out)
	}
	if !strings.Contains(out, "installed") {
		t.Errorf("install said:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dest, "example-adoption", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	out, code = run(t, bin, "skill", "list", "--dir", dest)
	if code != 0 || !strings.Contains(out, "up-to-date") {
		t.Errorf("list exited %d:\n%s", code, out)
	}

	out, code = run(t, bin, "skill", "nonsense")
	if code == 0 {
		t.Errorf("an unknown subcommand exited 0:\n%s", out)
	}
}

// The driver has to keep working. An interception that swallowed too much
// would show up as a non-zero exit here and nowhere else.
func TestDriverStillRuns(t *testing.T) {
	bin := build(t)

	out, code := run(t, bin, "./...")
	if code != 0 {
		t.Errorf("analysis run exited %d:\n%s", code, out)
	}

	out, code = run(t, bin, "-V=full")
	if code != 0 {
		t.Errorf("-V=full exited %d:\n%s", code, out)
	}
}

// `go vet -vettool=` drives the binary over the unitchecker protocol. It
// passes -flags and a config file path, never a subcommand.
func TestVettool(t *testing.T) {
	bin := build(t)

	out, code := run(t, "go", "vet", "-vettool="+bin, "./...")
	if code != 0 {
		t.Errorf("go vet -vettool exited %d:\n%s", code, out)
	}
}

// Uninstall through the real binary. Intercept exits the process itself, so
// this is the only place the destructive path runs the way a user meets it,
// and --dir is the only thing that says where it deletes from.
func TestSkillUninstall(t *testing.T) {
	bin := build(t)
	dest := filepath.Join(t.TempDir(), "skills")

	out, code := run(t, bin, "skill", "install", "--dir", dest)
	if code != 0 {
		t.Fatalf("install exited %d:\n%s", code, out)
	}

	out, code = run(t, bin, "skill", "remove", "--dir", dest)
	if code != 0 {
		t.Fatalf("remove exited %d:\n%s", code, out)
	}
	if !strings.Contains(out, "removed") {
		t.Errorf("remove said:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dest, "example-adoption")); err == nil {
		t.Error("the skill survived remove --dir")
	}

	// Removing what is no longer there is a skip, not a failure.
	out, code = run(t, bin, "skill", "uninstall", "--dir", dest)
	if code != 0 {
		t.Fatalf("a second uninstall exited %d:\n%s", code, out)
	}
	if !strings.Contains(out, "not installed") {
		t.Errorf("a second uninstall said:\n%s", out)
	}
}
