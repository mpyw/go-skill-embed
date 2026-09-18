// Package textfmt shapes strings for a terminal listing.
//
// Neither function knows anything about skills. They are here so that the
// files rendering the command's output do not each carry a copy.
package textfmt

import (
	"strings"
	"unicode/utf8"
)

// TrimLines removes the padding a tabwriter leaves at the end of a line when
// the last column is empty.
//
// Nothing should print trailing whitespace, and an Output comment in a Go
// example cannot carry it either.
func TrimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// Width is how much of a description a listing shows, counted in runes.
const Width = 100

// FirstLine shortens s to one line of at most Width runes.
//
// The count is in runes and the cut falls on a rune boundary. Bytes would cut
// a CJK description at a third of the length, and in the middle of a
// character. A tab becomes a space, because the caller feeds a tabwriter.
func FirstLine(s string) string {
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "\t", " ")
	if utf8.RuneCountInString(s) > Width {
		return string([]rune(s)[:Width-3]) + "..."
	}
	return s
}
