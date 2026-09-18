package skillembed

import "github.com/mpyw/go-skill-embed/internal/manifest"

// Frontmatter keys written into an installed SKILL.md. They let a later run
// tell an outdated copy from one that was edited by hand.
const (
	MetaKeyEmbeddedBy      = manifest.KeyEmbeddedBy
	MetaKeyEmbeddedVersion = manifest.KeyEmbeddedVersion
	MetaKeyEmbeddedAt      = manifest.KeyEmbeddedAt
	MetaKeyEmbeddedDigest  = manifest.KeyEmbeddedDigest
)
