package skillembed

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A description is cut for a listing. Counting bytes cuts a CJK description at
// a third of the length, and in the middle of a character.
func TestDescriptionTruncation(t *testing.T) {
	long := strings.Repeat("これは非常に長い日本語の説明文です。", 10)
	got := usageFirstLine(long)

	if !utf8.ValidString(got) {
		t.Errorf("cut in the middle of a rune: %q", got)
	}
	if n := utf8.RuneCountInString(got); n != usageWidth {
		t.Errorf("kept %d runes, want %d", n, usageWidth)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("no ellipsis: %q", got)
	}

	// A tab would be read as a column separator by the tabwriter this feeds.
	if got, want := usageFirstLine("before\tafter"), "before after"; got != want {
		t.Errorf("usageFirstLine(tab) = %q, want %q", got, want)
	}
	// A carriage return ends the line, the same as a newline.
	if got, want := usageFirstLine("first\r\nsecond"), "first"; got != want {
		t.Errorf("usageFirstLine(CRLF) = %q, want %q", got, want)
	}
	// Anything short is left alone.
	if got, want := usageFirstLine("short"), "short"; got != want {
		t.Errorf("usageFirstLine(short) = %q, want %q", got, want)
	}
}
