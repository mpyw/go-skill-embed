package textfmt

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A description is cut for a listing. Counting bytes cuts a CJK description at
// a third of the length, and in the middle of a character.
func TestFirstLine(t *testing.T) {
	const width = Width

	long := strings.Repeat("これは非常に長い日本語の説明文です。", 10)
	got := FirstLine(long)

	if !utf8.ValidString(got) {
		t.Errorf("cut in the middle of a rune: %q", got)
	}
	if n := utf8.RuneCountInString(got); n != width {
		t.Errorf("kept %d runes, want %d", n, width)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("no ellipsis: %q", got)
	}

	for _, c := range []struct{ in, want string }{
		// A tab would be read as a column separator by the tabwriter this feeds.
		{"before\tafter", "before after"},
		// A carriage return ends the line, the same as a newline.
		{"first\r\nsecond", "first"},
		{"first\nsecond", "first"},
		// Anything short is left alone.
		{"short", "short"},
		{"", ""},
	} {
		if got := FirstLine(c.in); got != c.want {
			t.Errorf("FirstLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTrimLines(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"a  \nb\t\nc", "a\nb\nc"},
		{"no padding", "no padding"},
		{"  leading is kept  ", "  leading is kept"},
		{"", ""},
	} {
		if got := TrimLines(c.in); got != c.want {
			t.Errorf("TrimLines(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
