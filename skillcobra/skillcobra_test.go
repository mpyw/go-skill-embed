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

//go:embed all:testdata/skills
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
