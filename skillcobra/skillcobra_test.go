package skillcobra_test

import (
	"bytes"
	"embed"
	"errors"
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

// --agent and --scope reach the installer. Without this the conversion in
// agentsValue and scopeValue could return nothing and the suite would pass,
// because every other test here names --dir and never resolves an agent.
func TestCommandAgentAndScope(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	fresh := func() (*cobra.Command, *bytes.Buffer) {
		in := skillembed.NewInstaller(
			skillembed.MustSkillsFromFS(testSkills, "testdata/skills"),
			skillembed.WithToolName("testtool"),
			skillembed.WithProjectRoot(root),
		)
		out := &bytes.Buffer{}
		cmd := &cobra.Command{Use: "testtool"}
		cmd.SetOut(out)
		cmd.SetErr(out)
		cmd.AddCommand(skillcobra.Command(in))
		return cmd, out
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
			cmd, out := fresh()
			cmd.SetArgs(append([]string{"skill", "install", "--dry-run"}, c.args...))
			if err := cmd.Execute(); err != nil {
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
	cmd, _ := fresh()
	cmd.SetArgs([]string{"skill", "install", "--agent", "bogus"})
	if err := cmd.Execute(); !errors.Is(err, skillembed.ErrUnknownAgent) {
		t.Errorf("err = %v, want it to wrap ErrUnknownAgent", err)
	}
	cmd, _ = fresh()
	cmd.SetArgs([]string{"skill", "install", "--scope", "bogus"})
	if err := cmd.Execute(); !errors.Is(err, skillembed.ErrUnknownScope) {
		t.Errorf("err = %v, want it to wrap ErrUnknownScope", err)
	}
}
