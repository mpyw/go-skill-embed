package skillembed

// AgentSelector names one agent, or a group of them, as --agent accepts it.
//
// The value space is every agent name the installer offers, plus the two
// group words below. An Agent value cannot carry those words, which is why
// this is its own type rather than []Agent.
type AgentSelector string

const (
	// AgentSelectorAll is every agent the installer offers, present or not.
	AgentSelectorAll AgentSelector = "all"
	// AgentSelectorDetected is the agents whose directory is already there.
	// It falls back to AgentSelectorAll when it finds none.
	AgentSelectorDetected AgentSelector = "detected"
)

// AgentSelectorFor names one agent.
//
//	skills.Install(ctx, skillembed.InstallOptions{
//		Agents: []skillembed.AgentSelector{
//			skillembed.AgentSelectorFor(skillembed.AgentClaudeCode),
//		},
//	})
func AgentSelectorFor(a Agent) AgentSelector { return AgentSelector(a.Name) }

// agentSelectorNames renders selectors for a message or a default.
//
//declscope:package // cli.go joins the same values for the --agent help and default
func agentSelectorNames(selectors []AgentSelector) []string {
	out := make([]string, len(selectors))
	for i, s := range selectors {
		out[i] = string(s)
	}
	return out
}
