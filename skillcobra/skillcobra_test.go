package skillcobra_test

import (
	"bytes"
	"embed"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	skillembed "github.com/mpyw/go-skill-embed"
	"github.com/mpyw/go-skill-embed/skillcobra"
)

//go:embed testdata/skills
var testSkills embed.FS

func TestCommand(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "skills")
	in := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(testSkills, "testdata/skills"),
		skillembed.WithToolName("testtool"),
	)

	out := &bytes.Buffer{}
	root := &cobra.Command{Use: "testtool"}
	root.SetOut(out)
	root.SetErr(out)
	root.AddCommand(skillcobra.Command(in))

	root.SetArgs([]string{"skill", "install", "--dir", dest})
	if err := root.Execute(); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if !strings.Contains(out.String(), "installed") {
		t.Errorf("install said:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dest, "demo-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	root.SetArgs([]string{"skill", "list", "--dir", dest})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(out.String(), "up-to-date") {
		t.Errorf("list said:\n%s", out)
	}
}

func ExampleCommand() {
	skills := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(testSkills, "testdata/skills"),
		skillembed.WithToolName("mytool"),
		skillembed.WithVersion("v0.1.0"),
	)

	root := &cobra.Command{Use: "mytool"}
	root.AddCommand(skillcobra.Command(skills))

	root.SetOut(os.Stdout)
	root.SetArgs([]string{"skill", "install", "--help"})
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}

	// Output:
	// Install the embedded skills
	//
	// Usage:
	//   mytool skill install [skill...] [flags]
	//
	// Flags:
	//       --agent strings   Target agent: {github-copilot|claude-code|cursor|codex|gemini|antigravity}, or all, or detected
	//       --dir string      Install to a custom directory (overrides --agent and --scope)
	//       --dry-run         Report what would happen without writing
	//   -f, --force           Overwrite existing skills
	//   -h, --help            help for install
	//       --scope string    Installation scope: {project|user} (default "project")
}

// Uninstall is the destructive one, and no adapter test drove it. --dir has to
// be the only thing that decides where it deletes from, so a second copy is
// installed at project scope: a --dir that reached the wrong option, or an
// alias wired to the wrong function, removes that one instead.
//
// It also covers the `remove` and `ls` aliases, which are reached here and
// nowhere else.
func TestUninstallAndAliases(t *testing.T) {
	project := t.TempDir()
	dest := filepath.Join(t.TempDir(), "skills")
	in := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(testSkills, "testdata/skills"),
		skillembed.WithToolName("testtool"),
		skillembed.WithProjectRoot(project),
	)

	// A fresh command tree per run, because cobra keeps the flag values it
	// parsed and a second Execute would inherit --dir from the first.
	run := func(args ...string) (string, error) {
		t.Helper()
		out := &bytes.Buffer{}
		root := &cobra.Command{Use: "testtool"}
		root.SetOut(out)
		root.SetErr(out)
		root.AddCommand(skillcobra.Command(in))
		root.SetArgs(args)
		err := root.Execute()
		return out.String(), err
	}

	if out, err := run("skill", "install", "--dir", dest); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if out, err := run("skill", "install", "--agent", "claude-code"); err != nil {
		t.Fatalf("install at project scope: %v\n%s", err, out)
	}
	decoy := filepath.Join(project, ".claude", "skills", "demo-skill")
	if _, err := os.Stat(decoy); err != nil {
		t.Fatalf("the project scope copy was never written, so the test proves nothing: %v", err)
	}

	out, err := run("skill", "remove", "--dir", dest)
	if err != nil {
		t.Fatalf("remove: %v\n%s", err, out)
	}
	if !strings.Contains(out, "removed") {
		t.Errorf("remove said:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dest, "demo-skill")); err == nil {
		t.Error("the skill survived remove --dir")
	}
	if _, err := os.Stat(decoy); err != nil {
		t.Errorf("remove --dir deleted from the agent directory instead: %v", err)
	}

	if out, err = run("skill", "ls", "--dir", dest); err != nil {
		t.Fatalf("ls: %v\n%s", err, out)
	}
	if !strings.Contains(out, "missing") {
		t.Errorf("ls said:\n%s", out)
	}
}

// --scope is the one flag bound through a type of this package's own, so it is
// the one that can accept a value the core would refuse, or refuse one it
// would accept. Every run here names --dir as well, so nothing can reach the
// real user directory whatever the scope says.
func TestScopeFlag(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "skills")
	in := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(testSkills, "testdata/skills"),
		skillembed.WithToolName("testtool"),
		skillembed.WithProjectRoot(t.TempDir()),
	)

	run := func(args ...string) (string, error) {
		t.Helper()
		out := &bytes.Buffer{}
		root := &cobra.Command{Use: "testtool"}
		root.SetOut(out)
		root.SetErr(out)
		root.AddCommand(skillcobra.Command(in))
		root.SetArgs(args)
		err := root.Execute()
		return out.String(), err
	}

	for _, scope := range []string{"project", "user"} {
		if out, err := run("skill", "list", "--dir", dest, "--scope", scope); err != nil {
			t.Errorf("--scope %s: %v\n%s", scope, err, out)
		}
	}

	out, err := run("skill", "list", "--dir", dest, "--scope", "nonsense")
	if err == nil {
		t.Fatalf("--scope nonsense was accepted:\n%s", out)
	}
	if !strings.Contains(err.Error(), "unknown scope") {
		t.Errorf("err = %v, want it to name the unknown scope", err)
	}

	// The bare command is help, not a failure and not an install.
	if out, err := run("skill"); err != nil {
		t.Errorf("bare skill: %v\n%s", err, out)
	} else if !strings.Contains(out, "install") {
		t.Errorf("bare skill said:\n%s", out)
	}
}

// Every front end prints the results before returning the error, because a run
// that wrote three skills and refused a fourth has to say so. An adapter that
// returned first would leave the user with a complaint and no report.
func TestResultsAreReportedBeforeTheError(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "skills")
	in := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(testSkills, "testdata/skills"),
		skillembed.WithToolName("testtool"),
		skillembed.WithProjectRoot(t.TempDir()),
	)

	out := &bytes.Buffer{}
	root := &cobra.Command{Use: "testtool"}
	root.SetOut(out)
	root.SetErr(out)
	root.AddCommand(skillcobra.Command(in))

	// Something else owns the name, so install has to refuse it.
	if err := os.MkdirAll(filepath.Join(dest, "demo-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "demo-skill", "SKILL.md"), []byte("hand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root.SetArgs([]string{"skill", "install", "--dir", dest})
	if err := root.Execute(); err == nil {
		t.Fatalf("install overwrote another tool's skill:\n%s", out)
	}
	if !strings.Contains(out.String(), "skipped") {
		t.Errorf("the report was not printed before the error:\n%s", out)
	}
	body, err := os.ReadFile(filepath.Join(dest, "demo-skill", "SKILL.md"))
	if err != nil || string(body) != "hand written\n" {
		t.Errorf("the blocked skill was overwritten: %q %v", body, err)
	}
}
