package skillembed

// Action is what install or uninstall did at one destination.
type Action string

const (
	ActionInstalled Action = "installed"
	ActionUpdated   Action = "updated"
	ActionRemoved   Action = "removed"
	ActionSkipped   Action = "skipped"
)
