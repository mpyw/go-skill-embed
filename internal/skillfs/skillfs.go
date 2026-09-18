// Package skillfs hashes and materialises a skill directory.
//
// The source is always an fs.FS, which is what //go:embed produces. embed.FS
// carries no file modes and no symlinks, so Write decides the mode itself and
// rejects anything that is not a regular file.
package skillfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/mpyw/go-skill-embed/internal/manifest"
)

// junkNames are files an operating system or a file browser leaves behind. A
// skill author never writes one on purpose.
var junkNames = map[string]bool{
	".DS_Store":   true,
	"Thumbs.db":   true,
	"desktop.ini": true,
	".localized":  true,
}

// IsJunk reports whether p is one of those files.
//
// They are skipped on the way in and on the way out. An installed skill sits
// in a directory a user may open in a file browser. A .DS_Store appearing
// beside it is not the user editing the skill.
func IsJunk(p string) bool { return junkNames[path.Base(p)] }

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
		if IsJunk(p) {
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
			data = manifest.Normalize(data)
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

// ExecutableBitsMatch reports whether every file carries the executable bit
// the rule asks for.
//
// The digest cannot answer this. An embedded file has no mode at all, so the
// two sides of a comparison never agree on one. The rule reads the contents
// instead. The digest has already matched those contents, so both sides reach
// the same answer.
func ExecutableBitsMatch(fsys fs.FS, rule func(name string, data []byte) bool) (bool, error) {
	if rule == nil {
		return true, nil
	}
	match := true
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || IsJunk(p) || !d.Type().IsRegular() || !match {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		if rule(p, data) != (info.Mode()&0o111 != 0) {
			match = false
		}
		return nil
	})
	return match, err
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
// The tree is staged in a sibling directory. The swap is two renames, with the
// old directory moved aside in between and moved back if the second rename
// fails. Removing the destination first is not atomic, so a failure part way
// through would leave dest with no installation at all.
func Write(ctx context.Context, src fs.FS, dest string, o WriteOptions) error {
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
		if err := ctx.Err(); err != nil {
			return err
		}
		target, err := safeJoin(staging, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if IsJunk(p) {
			return nil
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

	// A name to move the old directory to, next to it so the rename stays on
	// one file system. MkdirTemp is only how the name is reserved.
	aside, err := os.MkdirTemp(parent, "."+filepath.Base(dest)+".old-")
	if err != nil {
		return err
	}
	if err := os.Remove(aside); err != nil {
		return err
	}

	moved := false
	if _, err := os.Lstat(dest); err == nil {
		if err := os.Rename(dest, aside); err != nil {
			return err
		}
		moved = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	if err := os.Rename(staging, dest); err != nil {
		if moved {
			// Put back what was there. The caller is no worse off than before.
			_ = os.Rename(aside, dest)
		}
		return err
	}
	if moved {
		return os.RemoveAll(aside)
	}
	return nil
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
