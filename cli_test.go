package skillembed

import (
	"bytes"
	"embed"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

//go:embed testdata/skills
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
	ctx := t.Context()
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
		handled, err := in.cliIntercept(ctx, argv)
		if handled {
			t.Errorf("%q was intercepted", argv)
		}
		if err != nil {
			t.Errorf("%q: %v", argv, err)
		}
	}
}

func TestInterceptTakesTheSubcommand(t *testing.T) {
	ctx := t.Context()
	dest := filepath.Join(t.TempDir(), "skills")
	in := newCLIInstaller(t)

	handled, err := in.cliIntercept(ctx, []string{"mytool", "skill", "install", "--dir", dest, "demo-skill"})
	if !handled {
		t.Fatal("skill install was not intercepted")
	}
	if err != nil {
		t.Fatal(err)
	}

	handled, err = in.cliIntercept(ctx, []string{"mytool", "skill", "nonsense"})
	if !handled {
		t.Fatal("an unknown subcommand was passed through to the driver")
	}
	if err == nil {
		t.Error("an unknown subcommand did not produce an error")
	}

	// Bare `mytool skill` prints help, which is not a failure.
	handled, err = in.cliIntercept(ctx, []string{"mytool", "skill"})
	if !handled || err != nil {
		t.Errorf("bare skill: handled=%v err=%v", handled, err)
	}
}

func TestInterceptHonoursTheCommandName(t *testing.T) {
	ctx := t.Context()
	in := newCLIInstaller(t, WithCommandName("skills"))

	if handled, _ := in.cliIntercept(ctx, []string{"mytool", "skill"}); handled {
		t.Error("the default name was still intercepted")
	}
	if handled, _ := in.cliIntercept(ctx, []string{"mytool", "skills"}); !handled {
		t.Error("the configured name was not intercepted")
	}
}

// -h on a subcommand answers with the command's own frame around the flag
// package's own flag block. The frame is what the flag package cannot give,
// and the block is what it should not be asked to give up.
func TestSubcommandHelp(t *testing.T) {
	ctx := t.Context()
	out := &bytes.Buffer{}
	in := newCLIInstaller(t, WithOutput(out))

	if err := in.Run(ctx, []string{"install", "-h"}); !errors.Is(err, ErrHelp) {
		t.Fatalf("Run(install -h) = %v, want ErrHelp", err)
	}
	help := out.String()

	for _, want := range []string{
		"Install the agent skills embedded in testtool.",
		"  testtool skill install [flags] [skill...]",
		"  -agent value", // one dash, as a flag package tool prints
		"  -scope value", // a flag.Value, like -agent
		"(default project)",
		"demo-skill",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("help is missing %q:\n%s", want, help)
		}
	}
	// The flag package's own header names the FlagSet and not the tool.
	if strings.Contains(help, "Usage of skill install:") {
		t.Errorf("the flag package's own header leaked through:\n%s", help)
	}
}

func TestUsageHintCounts(t *testing.T) {
	in := newCLIInstaller(t)
	if got, want := in.UsageHint(), `Run "testtool skill" to install the 2 agent skills embedded in testtool.`; got != want {
		t.Errorf("UsageHint() = %q, want %q", got, want)
	}
}

// What was asked for goes to the output, and what went wrong goes to the error
// output. `mytool skill list > skills.txt` should put the list in the file and
// the complaint on the terminal, not the other way round.
func TestOutputAndErrorOutputAreSeparate(t *testing.T) {
	ctx := t.Context()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	in := newCLIInstaller(t, WithOutput(out), WithErrorOutput(errOut))

	reset := func() { out.Reset(); errOut.Reset() }

	// Help was asked for, so it is what was wanted.
	if err := in.Run(ctx, []string{"install", "-h"}); !errors.Is(err, ErrHelp) {
		t.Fatalf("Run(install -h) = %v, want ErrHelp", err)
	}
	if !strings.Contains(out.String(), "Install the agent skills") {
		t.Errorf("help did not reach the output:\n%s", out)
	}
	if errOut.Len() != 0 {
		t.Errorf("help reached the error output:\n%s", errOut)
	}

	// A bad flag is a mistake, and it is reported once.
	reset()
	if err := in.Run(ctx, []string{"install", "--bogus"}); err == nil {
		t.Fatal("a bad flag was accepted")
	}
	if out.Len() != 0 {
		t.Errorf("a diagnostic reached the output:\n%s", out)
	}
	if n := strings.Count(errOut.String(), "Usage:"); n != 1 {
		t.Errorf("usage printed %d times:\n%s", n, errOut)
	}

	reset()
	if err := in.Run(ctx, []string{"bogus"}); err == nil {
		t.Fatal("an unknown subcommand was accepted")
	}
	if out.Len() != 0 {
		t.Errorf("a diagnostic reached the output:\n%s", out)
	}
	if !strings.Contains(errOut.String(), "Usage:") {
		t.Errorf("no usage on the error output:\n%s", errOut)
	}
}

// Each subcommand's help is written out by hand, so the one place it can go
// wrong is a heading copied from the command above it. Telling a user that
// `uninstall` installs is worse than printing nothing.
func TestSubcommandHelpSaysWhichSubcommand(t *testing.T) {
	ctx := t.Context()

	for _, c := range []struct {
		sub, summary string
	}{
		{"install", "Install the agent skills embedded in testtool."},
		{"uninstall", "Remove the agent skills embedded in testtool."},
		{"list", "Show the agent skills embedded in testtool, and where each one stands."},
	} {
		t.Run(c.sub, func(t *testing.T) {
			out := &bytes.Buffer{}
			in := newCLIInstaller(t, WithOutput(out))
			if err := in.Run(ctx, []string{c.sub, "-h"}); !errors.Is(err, ErrHelp) {
				t.Fatalf("Run(%s -h) = %v, want ErrHelp", c.sub, err)
			}
			help := out.String()
			if !strings.HasPrefix(help, c.summary) {
				t.Errorf("help opens with:\n%s\nwant %q", help, c.summary)
			}
			if want := "  testtool skill " + c.sub + " [flags] [skill...]"; !strings.Contains(help, want) {
				t.Errorf("help is missing %q:\n%s", want, help)
			}
		})
	}
}
