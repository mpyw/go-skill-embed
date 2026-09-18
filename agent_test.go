package skillembed_test

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	skillembed "github.com/mpyw/go-skill-embed"
)

//go:embed testdata/skills
var agentTestSkills embed.FS

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

// Dir is asked for a directory before anything is written to it. Every answer
// it cannot give has to be an error, because an empty path joined with a skill
// name is a relative directory in the working directory.
func TestAgentDirRefusesWhatItCannotResolve(t *testing.T) {
	for _, c := range []struct {
		name  string
		agent skillembed.Agent
		scope skillembed.Scope
		want  string
	}{
		{"unknown scope", skillembed.AgentClaudeCode, "nonsense", `unknown scope "nonsense"`},
		{"no scope at all", skillembed.AgentClaudeCode, "", `unknown scope ""`},
		{"no user directory", skillembed.Agent{Name: "custom", ProjectDir: "x"}, skillembed.ScopeUser,
			"agent custom has no user scope directory"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, err := c.agent.Dir(c.scope, t.TempDir())
			if err == nil {
				t.Fatalf("Dir(%q) = %q, want an error", c.scope, dir)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to say %q", err, c.want)
			}
			if dir != "" {
				t.Errorf("Dir returned %q alongside an error", dir)
			}
		})
	}
}

// An empty project root means the working directory. Returning a relative path
// instead would install into whatever directory the tool happened to be run
// from later.
func TestAgentProjectDirDefaultsToTheWorkingDirectory(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := skillembed.AgentClaudeCode.Dir(skillembed.ScopeProject, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(wd, ".claude", "skills"); dir != want {
		t.Errorf("Dir = %q, want %q", dir, want)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("Dir = %q, want an absolute path", dir)
	}
}

// User scope is the home directory, and a machine without one cannot be
// guessed at. Falling back to a relative path would write .copilot/skills into
// the working directory.
func TestAgentUserDirNeedsAHomeDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	for _, a := range skillembed.DefaultAgents() {
		dir, err := a.Dir(skillembed.ScopeUser, "")
		if err == nil {
			t.Errorf("%s user dir = %q with no home directory, want an error", a.Name, dir)
		}
	}
}

// CLAUDE_CONFIG_DIR may hold several roots separated the way PATH is. Joining
// the whole value would produce one directory with a separator in its name.
func TestAgentClaudeConfigDirTakesTheFirstRoot(t *testing.T) {
	home, first, second := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)

	for _, c := range []struct {
		name  string
		value string
		want  string
	}{
		{"one root", first, filepath.Join(first, "skills")},
		{"several roots", first + string(filepath.ListSeparator) + second, filepath.Join(first, "skills")},
		// Nothing usable in the variable is the same as not setting it.
		{"empty", "", filepath.Join(home, ".claude", "skills")},
		{"whitespace", "   ", filepath.Join(home, ".claude", "skills")},
		{"an empty first root", string(filepath.ListSeparator) + second, filepath.Join(home, ".claude", "skills")},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", c.value)
			dir, err := skillembed.AgentClaudeCode.Dir(skillembed.ScopeUser, "")
			if err != nil {
				t.Fatal(err)
			}
			if dir != c.want {
				t.Errorf("Dir = %q, want %q", dir, c.want)
			}
		})
	}
}

// A selector is an agent name or one of the two group words. The constants and
// AgentSelectorFor are how a caller reaches both without writing a bare string.
func TestAgentSelectorsReachEveryForm(t *testing.T) {
	root := t.TempDir()
	in := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(agentTestSkills, "testdata/skills"),
		skillembed.WithToolName("testtool"),
		skillembed.WithProjectRoot(root),
	)

	for _, c := range []struct {
		name      string
		selectors []skillembed.AgentSelector
		want      int
	}{
		{"one agent", []skillembed.AgentSelector{skillembed.AgentSelectorFor(skillembed.AgentClaudeCode)}, 1},
		{"all", []skillembed.AgentSelector{skillembed.AgentSelectorAll}, 2},
		{"detected falls back to all", []skillembed.AgentSelector{skillembed.AgentSelectorDetected}, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			targets, err := in.Targets(skillembed.InstallOptions{Agents: c.selectors, Scope: skillembed.ScopeProject})
			if err != nil {
				t.Fatal(err)
			}
			if len(targets) != c.want {
				t.Errorf("got %d targets, want %d", len(targets), c.want)
			}
		})
	}

	if got := skillembed.AgentSelectorFor(skillembed.AgentClaudeCode); got != "claude-code" {
		t.Errorf("AgentSelectorFor(AgentClaudeCode) = %q, want the agent's name", got)
	}

	// The default is a selector too, and replacing it replaces detection.
	copilot := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(agentTestSkills, "testdata/skills"),
		skillembed.WithProjectRoot(root),
		skillembed.WithDefaultAgents(skillembed.AgentSelectorFor(skillembed.AgentGitHubCopilot)),
	)
	targets, err := copilot.Targets(skillembed.InstallOptions{Scope: skillembed.ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || len(targets[0].Agents) != 1 || targets[0].Agents[0].Name != "github-copilot" {
		t.Errorf("targets = %+v, want github-copilot alone", targets)
	}
}
