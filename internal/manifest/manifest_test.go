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

// A manifest with no frontmatter gains one on install, and a manifest with an
// empty one keeps it. Normalize has to make the two hash alike, or a skill
// reads as modified the moment it is installed.
func TestNormalizeMakesTheTwoEmptyFormsAgree(t *testing.T) {
	entries := []Entry{{Key: KeyEmbeddedBy, Value: "mytool"}, {Key: KeyEmbeddedDigest, Value: "sha256:abc"}}

	for _, src := range []string{
		"A manifest that carries no frontmatter at all.\n",
		"---\n---\nA manifest whose frontmatter block is empty.\n",
		"---\n\n  \n---\nA manifest whose frontmatter block is blank.\n",
	} {
		stamped := With([]byte(src), entries)
		if Fields(stamped)[KeyEmbeddedBy] != "mytool" {
			t.Errorf("metadata not readable back:\n%s", stamped)
		}
		if got, want := string(Normalize(stamped)), string(Normalize([]byte(src))); got != want {
			t.Errorf("the installed copy does not normalize to its source:\ngot:  %q\nwant: %q", got, want)
		}
	}
}

// A block scalar may hold a line that looks exactly like an injected key.
// Removing it would change the file the author wrote.
func TestStripLeavesNestedContentAlone(t *testing.T) {
	src := "---\nname: demo\nexample: |\n  " + KeyEmbeddedDigest + ": \"sha256:x\"\n  second line\n---\nbody\n"

	stamped := With([]byte(src), []Entry{{Key: KeyEmbeddedDigest, Value: "sha256:real"}})
	if !strings.Contains(string(stamped), "  "+KeyEmbeddedDigest+": \"sha256:x\"") {
		t.Errorf("the block scalar lost a line:\n%s", stamped)
	}
	if got, want := string(Strip(stamped)), src; got != want {
		t.Errorf("Strip did not restore the original:\ngot:  %q\nwant: %q", got, want)
	}
	// The real key is still the one that reads back.
	if got := Fields(stamped)[KeyEmbeddedDigest]; got != "sha256:real" {
		t.Errorf("%s = %q, want the injected value", KeyEmbeddedDigest, got)
	}
}
