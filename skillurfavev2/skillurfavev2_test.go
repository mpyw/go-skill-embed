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

//go:embed all:testdata/skills
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
	//    --agent value [ --agent value ]  Target agent: {github-copilot|claude-code|cursor|codex|gemini|antigravity}, or all
	//    --dir value                      Install to a custom directory (overrides --agent and --scope)
	//    --scope value                    Installation scope: {project|user} (default: "project")
	//    --force, -f                      Overwrite existing skills (default: false)
	//    --dry-run                        Report what would happen without writing (default: false)
	//    --help, -h                       show help
}
