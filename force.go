package skillembed

import (
	"errors"
	"fmt"
	"strings"
)

// NeedsForce reports whether overwriting this state would destroy work that
// this tool did not create.
func (s State) NeedsForce() bool { return s == StateModified || s == StateForeign }

// ErrNeedsForce reports that a destination held something this tool did not
// write, or something edited after it did. Install leaves those skills alone
// and reports the rest, so a caller branches on this rather than on the text:
//
//	results, err := skills.Install(o)
//	printed(results)
//	if errors.Is(err, skillembed.ErrNeedsForce) {
//		// tell the user to re-run with --force
//	}
var ErrNeedsForce = errors.New("skillembed: destination needs force")

// ForceRequiredError names every skill Install left alone.
type ForceRequiredError struct {
	// Blocked is the state of each destination that was not written.
	Blocked []InstallStatus
}

func (e *ForceRequiredError) Error() string {
	var b strings.Builder
	b.WriteString("refusing to overwrite skills this tool did not install, or that were edited after installing:\n")
	for _, st := range e.Blocked {
		fmt.Fprintf(&b, "  %s (%s)\n", st.Path, st.State)
	}
	b.WriteString("re-run with --force to overwrite")
	return b.String()
}

// Unwrap lets a caller match with errors.Is(err, ErrNeedsForce).
func (*ForceRequiredError) Unwrap() error { return ErrNeedsForce }
