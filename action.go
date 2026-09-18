package skillembed

// Action is what install or uninstall did at one destination.
type Action string

const (
	// ActionInstalled means the skill was written where nothing was.
	ActionInstalled Action = "installed"
	// ActionUpdated means an existing installation was replaced.
	ActionUpdated Action = "updated"
	// ActionRemoved means the skill directory was deleted.
	ActionRemoved Action = "removed"
	// ActionSkipped means nothing was done. Result.Reason says why.
	ActionSkipped Action = "skipped"
)
