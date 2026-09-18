package skillfs

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/mpyw/go-skill-embed/internal/manifest"
)

func demoFS() fstest.MapFS {
	return fstest.MapFS{
		manifest.FileName:   &fstest.MapFile{Data: []byte("---\nname: demo\n---\n\n# Demo\n")},
		"scripts/run.sh":    &fstest.MapFile{Data: []byte("#!/bin/sh\necho hi\n")},
		"reference/tips.md": &fstest.MapFile{Data: []byte("tips\n")},
	}
}

func TestDigestIgnoresInjectedMetadata(t *testing.T) {
	plain := demoFS()

	stamped := demoFS()
	stamped[manifest.FileName] = &fstest.MapFile{
		Data: manifest.With(plain[manifest.FileName].Data, []manifest.Entry{
			{Key: manifest.KeyEmbeddedBy, Value: "mytool"},
		}),
	}

	a, err := Digest(plain)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Digest(stamped)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("digest changed when metadata was injected:\n%s\n%s", a, b)
	}
}

func TestDigestNoticesAnEdit(t *testing.T) {
	before, err := Digest(demoFS())
	if err != nil {
		t.Fatal(err)
	}
	edited := demoFS()
	edited["reference/tips.md"] = &fstest.MapFile{Data: []byte("tips, edited\n")}
	after, err := Digest(edited)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Error("digest did not change after an edit")
	}
}

func TestWriteRestoresTheExecutableBit(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo")
	if err := Write(demoFS(), dest, WriteOptions{}); err != nil {
		t.Fatal(err)
	}

	script, err := os.Stat(filepath.Join(dest, "scripts", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if script.Mode().Perm()&0o111 == 0 {
		t.Errorf("scripts/run.sh mode = %v, want the executable bit set", script.Mode().Perm())
	}

	doc, err := os.Stat(filepath.Join(dest, "reference", "tips.md"))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Mode().Perm()&0o111 != 0 {
		t.Errorf("reference/tips.md mode = %v, want no executable bit", doc.Mode().Perm())
	}
}

func TestWriteReplacesWhatIsThere(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dest, "gone.md")
	if err := os.WriteFile(stale, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Write(demoFS(), dest, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("a file from the previous installation survived")
	}
}

func TestWriteTransforms(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo")
	err := Write(demoFS(), dest, WriteOptions{
		Transform: func(name string, data []byte) ([]byte, error) {
			if name != manifest.FileName {
				return data, nil
			}
			return append(data, []byte("appended\n")...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if want := "appended\n"; len(got) < len(want) || string(got[len(got)-len(want):]) != want {
		t.Errorf("Transform not applied:\n%s", got)
	}
}
