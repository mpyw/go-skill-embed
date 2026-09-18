package skillembed

import (
	"errors"
	"fmt"
)

// Scope selects where skills are installed.
type Scope string

const (
	// ScopeProject installs into the current project directory.
	ScopeProject Scope = "project"
	// ScopeUser installs into the user's home directory.
	ScopeUser Scope = "user"
)

// ErrUnknownScope reports a --scope value that is neither project nor user.
var ErrUnknownScope = errors.New("skillembed: unknown scope")

// ParseScope validates a scope name.
func ParseScope(s string) (Scope, error) {
	switch Scope(s) {
	case ScopeProject:
		return ScopeProject, nil
	case ScopeUser:
		return ScopeUser, nil
	}
	return "", fmt.Errorf("%w %q (want %s or %s)", ErrUnknownScope, s, ScopeProject, ScopeUser)
}
