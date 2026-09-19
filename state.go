package skillembed

// State describes what is already present at a destination.
type State string

const (
	// StateMissing means nothing is installed there yet.
	StateMissing State = "missing"
	// StateUpToDate means the installed copy matches the embedded skill.
	StateUpToDate State = "up-to-date"
	// StateOutdated means this tool installed it and it is not what the binary
	// would write now. The binary carries a newer copy, or a file lost the
	// executable bit it was installed with.
	StateOutdated State = "outdated"
	// StateModified means this tool installed it and the files were edited
	// afterwards.
	StateModified State = "modified"
	// StateForeign means something else owns a skill of that name there.
	StateForeign State = "foreign"
	// StateOrphaned means this tool wrote it, it is still byte for byte what
	// was written, and the binary no longer carries a skill of that name. An
	// earlier version installed it and nothing would reach the directory
	// again.
	StateOrphaned State = "orphaned"
)
