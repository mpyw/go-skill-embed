package skillembed

// State describes what is already present at a destination.
type State string

const (
	// StateMissing means nothing is installed there yet.
	StateMissing State = "missing"
	// StateUpToDate means the installed copy matches the embedded skill.
	StateUpToDate State = "up-to-date"
	// StateOutdated means we installed it and it is no longer what this binary
	// would write. The binary carries a newer copy, or a file lost the
	// executable bit it was installed with.
	StateOutdated State = "outdated"
	// StateModified means we installed it and the files were edited afterwards.
	StateModified State = "modified"
	// StateForeign means something else owns a skill of that name there.
	StateForeign State = "foreign"
)
