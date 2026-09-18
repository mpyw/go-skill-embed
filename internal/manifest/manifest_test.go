package manifest

import (
	"strings"
	"testing"
)

const withFrontmatter = `---
name: demo-skill
description: "A demo, with a colon: right here"
---

# Body
`

func TestFields(t *testing.T) {
	got := Fields([]byte(withFrontmatter))
	if got["name"] != "demo-skill" {
		t.Errorf("name = %q", got["name"])
	}
	if got["description"] != "A demo, with a colon: right here" {
		t.Errorf("description = %q", got["description"])
	}
}

func TestFieldsWithoutFrontmatter(t *testing.T) {
	if got := Fields([]byte("# Just a body\n")); got != nil {
		t.Errorf("Fields = %v, want nil", got)
	}
}

func TestFieldsSkipsNestedContent(t *testing.T) {
	src := "---\nname: demo\nallowed-tools:\n  - Read\n  - Bash\n---\n"
	got := Fields([]byte(src))
	if _, ok := got["- Read"]; ok {
		t.Errorf("sequence item read as a field: %v", got)
	}
	if got["name"] != "demo" {
		t.Errorf("name = %q", got["name"])
	}
}

func TestWithThenStripRoundTrips(t *testing.T) {
	entries := []Entry{{Key: KeyEmbeddedBy, Value: "mytool"}, {Key: KeyEmbeddedDigest, Value: "sha256:abc"}}

	stamped := With([]byte(withFrontmatter), entries)
	if !strings.Contains(string(stamped), KeyEmbeddedBy+": mytool") {
		t.Fatalf("metadata not written:\n%s", stamped)
	}
	if got := Fields(stamped)["name"]; got != "demo-skill" {
		t.Errorf("existing field lost: name = %q", got)
	}

	// Stamping twice must not accumulate duplicates.
	twice := With(stamped, entries)
	if n := strings.Count(string(twice), KeyEmbeddedBy+":"); n != 1 {
		t.Errorf("%s appears %d times, want 1:\n%s", KeyEmbeddedBy, n, twice)
	}

	if got, want := string(Strip(twice)), withFrontmatter; got != want {
		t.Errorf("Strip did not restore the original:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestWithCreatesFrontmatter(t *testing.T) {
	stamped := With([]byte("# Body only\n"), []Entry{{Key: KeyEmbeddedBy, Value: "mytool"}})
	if !strings.HasPrefix(string(stamped), "---\n") {
		t.Fatalf("no frontmatter added:\n%s", stamped)
	}
	if Fields(stamped)[KeyEmbeddedBy] != "mytool" {
		t.Errorf("field not readable back:\n%s", stamped)
	}
	if !strings.Contains(string(stamped), "# Body only") {
		t.Errorf("body lost:\n%s", stamped)
	}
}

// A manifest with no frontmatter gains one on install. Strip has to take it
// away again, or the installed copy never hashes equal to its source.
func TestWithThenStripRoundTripsWithoutFrontmatter(t *testing.T) {
	const src = "A manifest that carries no frontmatter at all.\n"
	entries := []Entry{{Key: KeyEmbeddedBy, Value: "mytool"}, {Key: KeyEmbeddedDigest, Value: "sha256:abc"}}

	stamped := With([]byte(src), entries)
	if Fields(stamped)[KeyEmbeddedBy] != "mytool" {
		t.Fatalf("metadata not readable back:\n%s", stamped)
	}
	if got := string(Strip(stamped)); got != src {
		t.Errorf("Strip did not restore the original:\ngot:  %q\nwant: %q", got, src)
	}
}
