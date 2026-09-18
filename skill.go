package skillembed

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/mpyw/go-skill-embed/internal/manifest"
	"github.com/mpyw/go-skill-embed/internal/skillfs"
)

// SkillFile is the manifest every skill directory must contain, as defined by
// the Agent Skills specification (https://agentskills.io/specification).
const SkillFile = manifest.FileName

// Skill is one embedded skill directory.
type Skill struct {
	// Name is the directory name the skill is installed under. It comes from
	// the `name` frontmatter field, falling back to the source directory name.
	Name string
	// Description is the `description` frontmatter field, if any.
	Description string
	// Dir is the skill's path inside the source file system.
	Dir string

	fsys   fs.FS
	digest string
}

// Digest is the SHA-256 of the skill's contents, recorded in the installed
// SKILL.md.
func (s Skill) Digest() string { return s.digest }

// FS returns a file system rooted at the skill directory.
func (s Skill) FS() (fs.FS, error) { return fs.Sub(s.fsys, s.Dir) }

// SkillSet is a collection of embedded skills, in name order.
type SkillSet struct {
	skills []Skill
}

// Skills returns the skills in the set.
func (s *SkillSet) Skills() []Skill {
	if s == nil {
		return nil
	}
	return append([]Skill(nil), s.skills...)
}

// Lookup finds a skill by name.
func (s *SkillSet) Lookup(name string) (Skill, bool) {
	if s != nil {
		for _, sk := range s.skills {
			if sk.Name == name {
				return sk, true
			}
		}
	}
	return Skill{}, false
}

// Names returns the skill names.
func (s *SkillSet) Names() []string {
	out := make([]string, 0, s.Len())
	for _, sk := range s.Skills() {
		out = append(out, sk.Name)
	}
	return out
}

// Len reports how many skills the set holds.
func (s *SkillSet) Len() int {
	if s == nil {
		return 0
	}
	return len(s.skills)
}

// SkillsFromFS discovers skills under root using the `<root>/*/SKILL.md` convention.
// If root itself holds a SKILL.md it is read as a single skill.
//
//	//go:embed skills
//	var skillsFS embed.FS
//
// A bare //go:embed leaves out every file whose name begins with a dot or an
// underscore, and says nothing about it. Write all:skills when a skill holds
// one. A skill carrying a file an operating system left behind, such as
// .DS_Store, is refused either way.
func SkillsFromFS(fsys fs.FS, root string) (*SkillSet, error) {
	if root == "" {
		root = "."
	}
	root = path.Clean(root)

	if _, err := fs.Stat(fsys, path.Join(root, SkillFile)); err == nil {
		sk, err := readSkill(fsys, root)
		if err != nil {
			return nil, err
		}
		return &SkillSet{skills: []Skill{sk}}, nil
	}

	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("skillembed: read %s: %w", root, err)
	}
	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := path.Join(root, e.Name())
		if _, err := fs.Stat(fsys, path.Join(dir, SkillFile)); err != nil {
			continue
		}
		sk, err := readSkill(fsys, dir)
		if err != nil {
			return nil, err
		}
		skills = append(skills, sk)
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("skillembed: no %s found under %s", SkillFile, root)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })

	seen := map[string]string{}
	for _, sk := range skills {
		if prev, ok := seen[sk.Name]; ok {
			return nil, fmt.Errorf("skillembed: %s and %s both declare the skill name %q", prev, sk.Dir, sk.Name)
		}
		seen[sk.Name] = sk.Dir
	}
	return &SkillSet{skills: skills}, nil
}

// MustSkillsFromFS is SkillsFromFS for a package level variable. It panics, which is what
// you want: a broken embed is a build-time mistake, not a runtime condition.
func MustSkillsFromFS(fsys fs.FS, root string) *SkillSet {
	set, err := SkillsFromFS(fsys, root)
	if err != nil {
		panic(err)
	}
	return set
}

func readSkill(fsys fs.FS, dir string) (Skill, error) {
	src, err := fs.ReadFile(fsys, path.Join(dir, SkillFile))
	if err != nil {
		return Skill{}, fmt.Errorf("skillembed: read %s: %w", path.Join(dir, SkillFile), err)
	}
	fields := manifest.Fields(src)
	name := fields["name"]
	if name == "" {
		name = path.Base(dir)
	}
	if err := checkSkillName(name); err != nil {
		return Skill{}, fmt.Errorf("skillembed: %s: %w", dir, err)
	}
	sk := Skill{
		Name:        name,
		Description: fields["description"],
		Dir:         dir,
		fsys:        fsys,
	}
	sub, err := sk.FS()
	if err != nil {
		return Skill{}, err
	}
	if err := checkSkillContents(sub, dir); err != nil {
		return Skill{}, err
	}
	if sk.digest, err = skillfs.Digest(sub); err != nil {
		return Skill{}, err
	}
	return sk, nil
}

// checkSkillContents refuses a skill that carries a file an operating system
// left behind. Installing is silent about them, because an installed directory
// is one a user may open in a file browser. An embedded one is different: it
// was committed, and it ships to everyone.
func checkSkillContents(fsys fs.FS, dir string) error {
	return fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !skillfs.IsJunk(p) {
			return err
		}
		return fmt.Errorf("skillembed: %s was left behind by an operating system,"+
			" and //go:embed all: took it along."+
			" Remove the file, or embed without the all: prefix", path.Join(dir, p))
	})
}

// checkSkillName rejects anything that could escape the destination directory
// or be hidden from the agent that reads it.
func checkSkillName(name string) error {
	switch {
	case name == "" || name == "." || name == "..":
		return fmt.Errorf("invalid skill name %q", name)
	case strings.ContainsAny(name, `/\`):
		return fmt.Errorf("skill name %q must not contain a path separator", name)
	case strings.HasPrefix(name, "."):
		return fmt.Errorf("skill name %q must not start with a dot", name)
	}
	return nil
}
