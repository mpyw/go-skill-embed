package skillurfavev3_test

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	skillembed "github.com/mpyw/go-skill-embed"
	"github.com/mpyw/go-skill-embed/skillurfavev3"
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
	app := &cli.Command{
		Name:     "testtool",
		Writer:   out,
		Commands: []*cli.Command{skillurfavev3.Command(in)},
	}

	if err := app.Run(context.Background(), []string{"testtool", "skill", "install", "--dir", dest}); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if !strings.Contains(out.String(), "installed") {
		t.Errorf("install said:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dest, "demo-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := app.Run(context.Background(), []string{"testtool", "skill", "list", "--dir", dest}); err != nil {
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

	app := &cli.Command{
		Name:     "mytool",
		Writer:   os.Stdout,
		Commands: []*cli.Command{skillurfavev3.Command(skills)},
	}

	if err := app.Run(context.Background(), []string{"mytool", "skill", "install", "--help"}); err != nil {
		log.Fatal(err)
	}

	// Output:
	// NAME:
	//    mytool skill install - Install the embedded skills
	//
	// USAGE:
	//    mytool skill install [options] [skill...]
	//
	// OPTIONS:
	//    --agent string [ --agent string ]  Target agent: {github-copilot|claude-code|cursor|codex|gemini|antigravity}, or all, or detected
	//    --dir string                       Install to a custom directory (overrides --agent and --scope)
	//    --scope string                     Installation scope: {project|user} (default: "project")
	//    --force, -f                        Overwrite or remove existing skills
	//    --dry-run                          Report what would happen without writing
	//    --help, -h                         show help
}

// --agent and --scope reach the installer. Without this the conversion in
// agentSelectors could return nothing and the suite would pass, because every
// other test here names --dir and never resolves an agent.
func TestCommandAgentAndScope(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	fresh := func() (*cli.Command, *bytes.Buffer) {
		in := skillembed.NewInstaller(
			skillembed.MustSkillsFromFS(testSkills, "testdata/skills"),
			skillembed.WithToolName("testtool"),
			skillembed.WithProjectRoot(root),
		)
		out := &bytes.Buffer{}
		return &cli.Command{
			Name:     "testtool",
			Writer:   out,
			Commands: []*cli.Command{skillurfavev3.Command(in)},
		}, out
	}

	for _, c := range []struct {
		name string
		args []string
		want []string
	}{
		{"one agent", []string{"--agent", "claude-code"}, []string{filepath.Join(root, ".claude", "skills")}},
		{"a comma separated list", []string{"--agent", "claude-code,cursor"}, []string{
			filepath.Join(root, ".claude", "skills"), filepath.Join(root, ".agents", "skills")}},
		{"a repeated flag", []string{"--agent", "claude-code", "--agent", "cursor"}, []string{
			filepath.Join(root, ".claude", "skills"), filepath.Join(root, ".agents", "skills")}},
		{"user scope", []string{"--scope", "user", "--agent", "claude-code"}, []string{
			filepath.Join(home, ".claude", "skills")}},
	} {
		t.Run(c.name, func(t *testing.T) {
			app, out := fresh()
			args := append([]string{"testtool", "skill", "install", "--dry-run"}, c.args...)
			if err := app.Run(t.Context(), args); err != nil {
				t.Fatalf("%v: %v\n%s", c.args, err, out)
			}
			for _, want := range c.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output does not name %s:\n%s", want, out)
				}
			}
			if n := strings.Count(out.String(), "would be installed"); n != len(c.want) {
				t.Errorf("got %d destinations, want %d:\n%s", n, len(c.want), out)
			}
		})
	}

	// An unknown value still fails, and the sentinel still reaches the caller.
	app, _ := fresh()
	if err := app.Run(t.Context(), []string{"testtool", "skill", "install", "--agent", "bogus"}); !errors.Is(err, skillembed.ErrUnknownAgent) {
		t.Errorf("err = %v, want it to wrap ErrUnknownAgent", err)
	}
	app, _ = fresh()
	if err := app.Run(t.Context(), []string{"testtool", "skill", "install", "--scope", "bogus"}); !errors.Is(err, skillembed.ErrUnknownScope) {
		t.Errorf("err = %v, want it to wrap ErrUnknownScope", err)
	}
}
