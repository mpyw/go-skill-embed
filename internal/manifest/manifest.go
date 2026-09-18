// Package manifest reads and rewrites the frontmatter of a SKILL.md.
//
// It deliberately does not parse YAML. The installer needs a handful of top
// level scalars and needs to rewrite its own keys without disturbing anything
// else, and a byte level edit preserves the rest of the file exactly.
package manifest

import (
	"bytes"
	"strings"
)

// FileName is the manifest every skill directory must contain, as defined by
// the Agent Skills specification (https://agentskills.io/specification).
const FileName = "SKILL.md"

// Keys written into an installed manifest. They are namespaced so they never
// collide with the source tracking keys `gh skill install` writes.
const (
	KeyEmbeddedBy      = "x-embedded-by"
	KeyEmbeddedVersion = "x-embedded-version"
	KeyEmbeddedAt      = "x-embedded-at"
	KeyEmbeddedDigest  = "x-embedded-digest"
)

// InjectedKeys are the keys With writes and Strip removes.
var InjectedKeys = []string{KeyEmbeddedBy, KeyEmbeddedVersion, KeyEmbeddedAt, KeyEmbeddedDigest}

// Entry is one key and value to inject.
type Entry struct {
	Key   string
	Value string
}

var delim = []byte("---")

// block locates the YAML frontmatter at the top of a manifest.
type block struct {
	// found reports whether the file opens with a --- delimited block.
	found bool
	// open is the offset of the opening delimiter line.
	open int
	// start and end bound the YAML, excluding the delimiter lines.
	start, end int
	// after is the offset just past the closing delimiter line.
	after int
}

func locate(src []byte) block {
	rest := bytes.TrimPrefix(src, []byte{0xEF, 0xBB, 0xBF}) // tolerate a BOM
	offset := len(src) - len(rest)

	line, next := readLine(rest, 0)
	if !isDelim(line) {
		return block{}
	}
	start := next
	for pos := next; pos < len(rest); {
		line, closed := readLine(rest, pos)
		if isDelim(line) {
			return block{
				found: true,
				open:  offset,
				start: offset + start,
				end:   offset + pos,
				after: offset + closed,
			}
		}
		pos = closed
	}
	return block{}
}

func isDelim(line []byte) bool {
	return bytes.Equal(bytes.TrimRight(line, " \t\r"), delim)
}

// readLine returns the line starting at pos, without its newline, and the
// offset of the line after it.
func readLine(src []byte, pos int) (line []byte, next int) {
	i := bytes.IndexByte(src[pos:], '\n')
	if i < 0 {
		return src[pos:], len(src)
	}
	return src[pos : pos+i], pos + i + 1
}

// Fields reads the top level `key: value` scalars. Nested mappings, sequences
// and block scalars are skipped rather than misread.
func Fields(src []byte) map[string]string {
	b := locate(src)
	if !b.found {
		return nil
	}
	fields := map[string]string{}
	body := src[b.start:b.end]
	for pos := 0; pos < len(body); {
		line, next := readLine(body, pos)
		pos = next
		text := strings.TrimRight(string(line), " \t\r")
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if text[0] == ' ' || text[0] == '\t' || text[0] == '-' {
			continue // nested content
		}
		key, value, ok := strings.Cut(text, ":")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		fields[key] = unquote(value)
	}
	return fields
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// quote wraps a value in double quotes when plain style would be ambiguous.
func quote(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":#\n\"'{}[],&*?|<>=!%@`") || s != strings.TrimSpace(s) {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
	}
	return s
}

// With returns src with the injected keys replaced by entries. A manifest
// without frontmatter gains one.
func With(src []byte, entries []Entry) []byte {
	src = Strip(src)
	b := locate(src)

	var added bytes.Buffer
	for _, e := range entries {
		added.WriteString(e.Key)
		added.WriteString(": ")
		added.WriteString(quote(e.Value))
		added.WriteByte('\n')
	}

	var out bytes.Buffer
	if !b.found {
		// No blank line after the closing delimiter. Strip has to restore the
		// file byte for byte, and a line it did not write is a line it cannot
		// know to remove.
		out.WriteString("---\n")
		out.Write(added.Bytes())
		out.WriteString("---\n")
		out.Write(src)
		return out.Bytes()
	}
	out.Write(src[:b.end])
	if b.end > b.start && src[b.end-1] != '\n' {
		out.WriteByte('\n')
	}
	out.Write(added.Bytes())
	out.Write(src[b.end:])
	return out.Bytes()
}

// Strip removes the injected keys, so an installed manifest can be compared
// against the embedded original.
func Strip(src []byte) []byte {
	b := locate(src)
	if !b.found {
		return src
	}
	body := src[b.start:b.end]
	var kept bytes.Buffer
	dropped := false
	for pos := 0; pos < len(body); {
		line, next := readLine(body, pos)
		start, end := pos, next
		pos = next
		if isInjected(line) {
			dropped = true
			continue
		}
		kept.Write(body[start:end])
	}
	if !dropped {
		return src
	}
	var out bytes.Buffer
	if len(bytes.TrimSpace(kept.Bytes())) == 0 {
		// Nothing but the injected keys was in there, so With wrote the block
		// itself. Removing the keys has to remove it too, or the manifest
		// never hashes equal to the skill it came from again.
		out.Write(src[:b.open])
		out.Write(src[b.after:])
		return out.Bytes()
	}
	out.Write(src[:b.start])
	out.Write(kept.Bytes())
	out.Write(src[b.end:])
	return out.Bytes()
}

func isInjected(line []byte) bool {
	key, _, ok := strings.Cut(strings.TrimRight(string(line), " \t\r"), ":")
	if !ok {
		return false
	}
	key = strings.TrimSpace(key)
	for _, k := range InjectedKeys {
		if key == k {
			return true
		}
	}
	return false
}
