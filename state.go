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
	// StateOrphaned means this tool installed it and the binary no longer
	// carries it. An earlier version embedded a skill that has since been
	// renamed or dropped, and nothing else would ever reach the directory
	// again.
	StateOrphaned State = "orphaned"
)
