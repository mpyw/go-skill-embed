package skillembed

import "github.com/mpyw/go-skill-embed/internal/manifest"

// Frontmatter keys written into an installed SKILL.md. They let a later run
// tell an outdated copy from one that was edited by hand.
const (
	MetaKeyEmbeddedBy = manifest.KeyEmbeddedBy
	// MetaKeyEmbeddedName is the skill this directory holds, which is also the
	// directory's own name when this tool put it there. A copy the user made
	// under another name disagrees, and that is how the two are told apart.
	MetaKeyEmbeddedName    = manifest.KeyEmbeddedName
	MetaKeyEmbeddedVersion = manifest.KeyEmbeddedVersion
	MetaKeyEmbeddedAt      = manifest.KeyEmbeddedAt
	MetaKeyEmbeddedDigest  = manifest.KeyEmbeddedDigest
)
