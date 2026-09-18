// Package skillfs hashes and materialises a skill directory.
//
// The source is always an fs.FS, which is what //go:embed produces. embed.FS
// carries no file modes and no symlinks, so Write decides the mode itself and
// rejects anything that is not a regular file.
package skillfs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/mpyw/go-skill-embed/internal/manifest"
)

// Digest is the SHA-256 of a skill directory: every regular file's path, size
// and contents, in path order.
//
// The manifest is hashed with the injected metadata removed, so a directory
// installed from an embedded skill hashes equal to the skill itself.
func Digest(fsys fs.FS) (string, error) {
	h := sha256.New()
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: not a regular file (%s)", p, d.Type())
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		if p == manifest.FileName {
			data = manifest.Strip(data)
		}
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", p, len(data))
		_, _ = h.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// WriteOptions configures Write.
type WriteOptions struct {
	// Transform rewrites a file's contents on the way out. A nil Transform
	// copies the file unchanged.
	Transform func(name string, data []byte) ([]byte, error)
	// Executable decides which files get the executable bit, which embed.FS
	// cannot carry. A nil Executable marks files starting with a #! shebang.
	Executable func(name string, data []byte) bool
}

// Write materialises src at dest, replacing whatever is there.
//
// The tree is staged in a sibling directory and swapped in with a rename, so a
// failure part way through leaves the existing installation untouched.
func Write(src fs.FS, dest string, o WriteOptions) error {
	executable := o.Executable
	if executable == nil {
		executable = HasShebang
	}

	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(dest)+".tmp-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staging) }()

	err = fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target, err := safeJoin(staging, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: not a regular file (%s)", p, d.Type())
		}
		data, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		if o.Transform != nil {
			if data, err = o.Transform(p, data); err != nil {
				return err
			}
		}
		mode := fs.FileMode(0o644)
		if executable(p, data) {
			mode = 0o755
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, mode)
	})
	if err != nil {
		return err
	}

	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return os.Rename(staging, dest)
}

// HasShebang is the default Executable test.
func HasShebang(_ string, data []byte) bool { return bytes.HasPrefix(data, []byte("#!")) }

// safeJoin rejects a path that would escape root.
func safeJoin(root, p string) (string, error) {
	if p == "." {
		return root, nil
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf("unsafe path %q in embedded skill", p)
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}
