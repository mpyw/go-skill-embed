package skillembed

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Agent is a coding agent that reads skills from a known directory.
//
// The built-in agents mirror the directories used by `gh skill install`.
type Agent struct {
	// Name is the flag value, e.g. "claude-code".
	Name string
	// Title is the human readable name, e.g. "Claude Code".
	Title string
	// ProjectDir is the skills directory relative to the project root.
	ProjectDir string
	// UserDir returns the absolute skills directory for user scope.
	UserDir func() (string, error)
}

// Dir resolves the skills directory for the given scope.
// projectRoot is only used for ScopeProject; an empty value means the
// current working directory.
func (a Agent) Dir(scope Scope, projectRoot string) (string, error) {
	switch scope {
	case ScopeProject:
		if projectRoot == "" {
			wd, err := os.Getwd()
			if err != nil {
				return "", err
			}
			projectRoot = wd
		}
		return filepath.Join(projectRoot, filepath.FromSlash(a.ProjectDir)), nil
	case ScopeUser:
		if a.UserDir == nil {
			return "", fmt.Errorf("agent %s has no user scope directory", a.Name)
		}
		return a.UserDir()
	}
	return "", fmt.Errorf("unknown scope %q", scope)
}

// sharedAgentProjectDir is the directory agreed on by every agent except Claude Code.
const sharedAgentProjectDir = ".agents/skills"

func agentHomeDir(parts ...string) func() (string, error) {
	return func() (string, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(append([]string{home}, parts...)...), nil
	}
}

// agentEnvOrHomeDir prefers an explicit config directory from the environment and
// falls back to a path under the home directory.
func agentEnvOrHomeDir(env string, envParts []string, homeParts ...string) func() (string, error) {
	return func() (string, error) {
		if root := strings.TrimSpace(os.Getenv(env)); root != "" {
			// A config dir may hold several colon separated roots; use the first.
			if i := strings.IndexByte(root, filepath.ListSeparator); i >= 0 {
				root = root[:i]
			}
			if root != "" {
				return filepath.Join(append([]string{root}, envParts...)...), nil
			}
		}
		return agentHomeDir(homeParts...)()
	}
}

// Built-in agents. The directories match `gh skill install`.
var (
	AgentGitHubCopilot = Agent{
		Name:       "github-copilot",
		Title:      "GitHub Copilot",
		ProjectDir: sharedAgentProjectDir,
		UserDir:    agentHomeDir(".copilot", "skills"),
	}
	AgentClaudeCode = Agent{
		Name:       "claude-code",
		Title:      "Claude Code",
		ProjectDir: ".claude/skills",
		// Claude Code relocates its whole config with CLAUDE_CONFIG_DIR.
		UserDir: agentEnvOrHomeDir("CLAUDE_CONFIG_DIR", []string{"skills"}, ".claude", "skills"),
	}
	AgentCursor = Agent{
		Name:       "cursor",
		Title:      "Cursor",
		ProjectDir: sharedAgentProjectDir,
		UserDir:    agentHomeDir(".cursor", "skills"),
	}
	AgentCodex = Agent{
		Name:       "codex",
		Title:      "Codex",
		ProjectDir: sharedAgentProjectDir,
		UserDir:    agentHomeDir(".codex", "skills"),
	}
	AgentGemini = Agent{
		Name:       "gemini",
		Title:      "Gemini CLI",
		ProjectDir: sharedAgentProjectDir,
		UserDir:    agentHomeDir(".gemini", "skills"),
	}
	AgentAntigravity = Agent{
		Name:       "antigravity",
		Title:      "Antigravity",
		ProjectDir: sharedAgentProjectDir,
		UserDir:    agentHomeDir(".gemini", "antigravity", "skills"),
	}
)

// DefaultAgents lists every built-in agent, in the order `gh skill install`
// documents them.
func DefaultAgents() []Agent {
	return []Agent{AgentGitHubCopilot, AgentClaudeCode, AgentCursor, AgentCodex, AgentGemini, AgentAntigravity}
}

// AgentChoices renders the --agent help string, such as {a|b|c}. An adapter
// that writes its own flag help uses it to name the same agents.
func (in *Installer) AgentChoices() string { return agentChoices(in.Agents()) }

// agentsFallBackToAll is the answer when "detected" found nothing. A machine
// with no agent directory is one where any guess is as good as another, and
// doing nothing would read as a failure.
//
//declscope:package // install.go applies it after resolving the agents
func agentsFallBackToAll(known, resolved []Agent, values []string) []Agent {
	if len(resolved) > 0 {
		return resolved
	}
	for _, v := range values {
		for _, name := range strings.Split(v, ",") {
			if strings.TrimSpace(name) == "detected" {
				return known
			}
		}
	}
	return resolved
}

// agentChoices renders the --agent help string, such as {a|b|c}.
func agentChoices(agents []Agent) string {
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.Name
	}
	return "{" + strings.Join(names, "|") + "}"
}

// agentsByName maps --agent values to agents.
//
// "all" expands to every agent the installer offers. "detected" keeps the ones
// whose directory already exists, and expands to all when that finds nothing,
// so the command still does something on a machine with no agent set up.
//
//declscope:package // the command line's agent vocabulary, read by install.go
func agentsByName(known []Agent, values []string, detected func(Agent) bool) ([]Agent, error) {
	if len(values) == 0 {
		return nil, nil
	}
	byName := make(map[string]Agent, len(known))
	for _, a := range known {
		byName[a.Name] = a
	}
	var out []Agent
	seen := map[string]bool{}
	for _, v := range values {
		for _, name := range strings.Split(v, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if name == "all" || name == "detected" {
				for _, a := range known {
					if name == "detected" && detected != nil && !detected(a) {
						continue
					}
					if !seen[a.Name] {
						seen[a.Name] = true
						out = append(out, a)
					}
				}
				continue
			}
			a, ok := byName[name]
			if !ok {
				valid := make([]string, 0, len(known))
				for _, k := range known {
					valid = append(valid, k.Name)
				}
				sort.Strings(valid)
				return nil, fmt.Errorf("unknown agent %q (want one of %s, or all, or detected)", name, strings.Join(valid, ", "))
			}
			if !seen[a.Name] {
				seen[a.Name] = true
				out = append(out, a)
			}
		}
	}
	return out, nil
}
