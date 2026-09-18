package skillembed_test

import (
	"path/filepath"
	"strings"
	"testing"

	skillembed "github.com/mpyw/go-skill-embed"
)

// The built-in agents are this module's central data, taken from
// `gh skill install --help`. A rename pass has already corrupted the titles
// once, and nothing else in the repository reads them closely enough to fail.
func TestBuiltInAgents(t *testing.T) {
	want := []struct {
		name, title, projectDir, userDir string
	}{
		{"github-copilot", "GitHub Copilot", ".agents/skills", ".copilot/skills"},
		{"claude-code", "Claude Code", ".claude/skills", ".claude/skills"},
		{"cursor", "Cursor", ".agents/skills", ".cursor/skills"},
		{"codex", "Codex", ".agents/skills", ".codex/skills"},
		{"gemini", "Gemini CLI", ".agents/skills", ".gemini/skills"},
		{"antigravity", "Antigravity", ".agents/skills", ".gemini/antigravity/skills"},
	}

	got := skillembed.DefaultAgents()
	if len(got) != len(want) {
		t.Fatalf("DefaultAgents() has %d agents, want %d", len(got), len(want))
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	for i, a := range got {
		w := want[i]
		if a.Name != w.name {
			t.Errorf("agent %d is %q, want %q", i, a.Name, w.name)
			continue
		}
		if a.Title != w.title {
			t.Errorf("%s title = %q, want %q", a.Name, a.Title, w.title)
		}
		if a.ProjectDir != w.projectDir {
			t.Errorf("%s project dir = %q, want %q", a.Name, a.ProjectDir, w.projectDir)
		}
		dir, err := a.Dir(skillembed.ScopeUser, "")
		if err != nil {
			t.Errorf("%s user dir: %v", a.Name, err)
			continue
		}
		if got, want := dir, filepath.Join(home, filepath.FromSlash(w.userDir)); got != want {
			t.Errorf("%s user dir = %q, want %q", a.Name, got, want)
		}
	}
}

// A title is what Label prints, so a rename landing in one is visible output.
func TestAgentTitlesAreNotIdentifiers(t *testing.T) {
	for _, a := range skillembed.DefaultAgents() {
		if strings.HasPrefix(a.Title, "Agent") {
			t.Errorf("%s title is %q, which reads as a Go identifier", a.Name, a.Title)
		}
	}
}

// Claude Code moves its whole configuration with CLAUDE_CONFIG_DIR.
func TestClaudeConfigDirIsHonoured(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)

	dir, err := skillembed.AgentClaudeCode.Dir(skillembed.ScopeUser, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "skills"); dir != want {
		t.Errorf("user dir = %q, want %q", dir, want)
	}
}
