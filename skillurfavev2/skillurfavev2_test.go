package skillurfavev2_test

import (
	"bytes"
	"embed"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"

	skillembed "github.com/mpyw/go-skill-embed"
	"github.com/mpyw/go-skill-embed/skillurfavev2"
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
	app := &cli.App{
		Name:     "testtool",
		Writer:   out,
		Commands: []*cli.Command{skillurfavev2.Command(in)},
	}

	if err := app.Run([]string{"testtool", "skill", "install", "--dir", dest}); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if !strings.Contains(out.String(), "installed") {
		t.Errorf("install said:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dest, "demo-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := app.Run([]string{"testtool", "skill", "list", "--dir", dest}); err != nil {
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

	app := &cli.App{
		Name:     "mytool",
		Writer:   os.Stdout,
		Commands: []*cli.Command{skillurfavev2.Command(skills)},
	}

	if err := app.Run([]string{"mytool", "skill", "install", "--help"}); err != nil {
		log.Fatal(err)
	}

	// Output:
	// NAME:
	//    mytool skill install - Install the embedded skills
	//
	// USAGE:
	//    mytool skill install [command options] [skill...]
	//
	// OPTIONS:
	//    --agent value [ --agent value ]  Target agent: {github-copilot|claude-code|cursor|codex|gemini|antigravity}, or all, or detected
	//    --dir value                      Install to a custom directory (overrides --agent and --scope)
	//    --scope value                    Installation scope: {project|user} (default: "project")
	//    --force, -f                      Overwrite existing skills (default: false)
	//    --dry-run                        Report what would happen without writing (default: false)
	//    --help, -h                       show help
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

	// A fresh app per run, so that nothing a previous parse left behind can
	// stand in for the flag under test.
	run := func(args ...string) (string, error) {
		t.Helper()
		out := &bytes.Buffer{}
		app := &cli.App{
			Name:     "testtool",
			Writer:   out,
			Commands: []*cli.Command{skillurfavev2.Command(in)},
		}
		err := app.Run(append([]string{"testtool"}, args...))
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

	run := func(args ...string) (string, error) {
		t.Helper()
		out := &bytes.Buffer{}
		app := &cli.App{
			Name:     "testtool",
			Writer:   out,
			Commands: []*cli.Command{skillurfavev2.Command(in)},
		}
		err := app.Run(append([]string{"testtool"}, args...))
		return out.String(), err
	}

	// Something else owns the name, so install has to refuse it.
	if err := os.MkdirAll(filepath.Join(dest, "demo-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "demo-skill", "SKILL.md"), []byte("hand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run("skill", "install", "--dir", dest)
	if err == nil {
		t.Fatalf("install overwrote another tool's skill:\n%s", out)
	}
	if !strings.Contains(out, "skipped") {
		t.Errorf("the report was not printed before the error:\n%s", out)
	}
	body, readErr := os.ReadFile(filepath.Join(dest, "demo-skill", "SKILL.md"))
	if readErr != nil || string(body) != "hand written\n" {
		t.Errorf("the blocked skill was overwritten: %q %v", body, readErr)
	}

	// An option the core refuses reaches the framework as an error rather than
	// an empty listing.
	if out, err := run("skill", "list", "--dir", dest, "--scope", "nonsense"); err == nil {
		t.Errorf("--scope nonsense was accepted:\n%s", out)
	}
}
