package skillembed

import (
	"bytes"
	"embed"
	"path/filepath"
	"testing"
)

//go:embed all:testdata/skills
var cliTestSkills embed.FS

func newCLIInstaller(t *testing.T, opts ...InstallerOption) *Installer {
	t.Helper()
	base := []InstallerOption{
		WithToolName("testtool"),
		WithProjectRoot(t.TempDir()),
		WithOutput(&bytes.Buffer{}),
	}
	return NewInstaller(MustSkillsFromFS(cliTestSkills, "testdata/skills"), append(base, opts...)...)
}

// Intercept must leave a driver's own command line alone. singlechecker,
// multichecker and unitchecker treat every non-flag argument as a package
// pattern, and `go vet -vettool=` passes -flags or a config file path.
func TestInterceptLeavesTheDriverAlone(t *testing.T) {
	in := newCLIInstaller(t)

	for _, argv := range [][]string{
		{"mytool"},
		{"mytool", "./..."},
		{"mytool", "-flags"},
		{"mytool", "-V=full"},
		{"mytool", "/tmp/vet123.cfg"},
		{"mytool", "skills"},  // near miss on the command name
		{"mytool", "-skill"},  // a flag, not the subcommand
		{"mytool", "./skill"}, // a package pattern that contains it
	} {
		handled, err := in.cliIntercept(argv)
		if handled {
			t.Errorf("%q was intercepted", argv)
		}
		if err != nil {
			t.Errorf("%q: %v", argv, err)
		}
	}
}

func TestInterceptTakesTheSubcommand(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "skills")
	in := newCLIInstaller(t)

	handled, err := in.cliIntercept([]string{"mytool", "skill", "install", "--dir", dest, "demo-skill"})
	if !handled {
		t.Fatal("skill install was not intercepted")
	}
	if err != nil {
		t.Fatal(err)
	}

	handled, err = in.cliIntercept([]string{"mytool", "skill", "nonsense"})
	if !handled {
		t.Fatal("an unknown subcommand was passed through to the driver")
	}
	if err == nil {
		t.Error("an unknown subcommand did not produce an error")
	}

	// Bare `mytool skill` prints help, which is not a failure.
	handled, err = in.cliIntercept([]string{"mytool", "skill"})
	if !handled || err != nil {
		t.Errorf("bare skill: handled=%v err=%v", handled, err)
	}
}

func TestInterceptHonoursTheCommandName(t *testing.T) {
	in := newCLIInstaller(t, WithCommandName("skills"))

	if handled, _ := in.cliIntercept([]string{"mytool", "skill"}); handled {
		t.Error("the default name was still intercepted")
	}
	if handled, _ := in.cliIntercept([]string{"mytool", "skills"}); !handled {
		t.Error("the configured name was not intercepted")
	}
}
