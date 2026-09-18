package skillembed

// State describes what is already present at a destination.
type State string

const (
	// StateMissing means nothing is installed there yet.
	StateMissing State = "missing"
	// StateUpToDate means the installed copy matches the embedded skill.
	StateUpToDate State = "up-to-date"
	// StateOutdated means we installed it and the binary now carries a newer copy.
	StateOutdated State = "outdated"
	// StateModified means we installed it and the files were edited afterwards.
	StateModified State = "modified"
	// StateForeign means something else owns a skill of that name there.
	StateForeign State = "foreign"
)

// NeedsForce reports whether overwriting this state would destroy work that
// this tool did not create.
func (s State) NeedsForce() bool { return s == StateModified || s == StateForeign }
