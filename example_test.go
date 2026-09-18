package skillembed_test

import (
	"embed"
	"errors"
	"fmt"
	"log"
	"os"

	skillembed "github.com/mpyw/go-skill-embed"
)

// The all: prefix keeps files whose names begin with a dot or an underscore.
// A bare //go:embed drops them, and says nothing about it.
//
//go:embed all:testdata/skills
var exampleSkills embed.FS

func ExampleMustSkillsFromFS() {
	set := skillembed.MustSkillsFromFS(exampleSkills, "testdata/skills")

	for _, sk := range set.Skills() {
		fmt.Println(sk.Name)
	}
	// Output:
	// bare-skill
	// demo-skill
}

func ExampleInstaller_Install() {
	skills := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(exampleSkills, "testdata/skills"),
		skillembed.WithToolName("mytool"),
		skillembed.WithVersion("v0.1.0"),
	)

	dir, err := os.MkdirTemp("", "skills")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	results, err := skills.Install(skillembed.InstallOptions{
		Dir:   dir,
		Names: []string{"demo-skill"},
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		fmt.Println(r.Action, r.Skill.Name)
	}

	// Installing again writes nothing, because the digest recorded in the
	// installed SKILL.md still matches.
	results, err = skills.Install(skillembed.InstallOptions{
		Dir:   dir,
		Names: []string{"demo-skill"},
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range results {
		fmt.Println(r.Action, r.Skill.Name, "-", r.Reason)
	}

	// Output:
	// installed demo-skill
	// skipped demo-skill - already up to date
}

func ExampleInstaller_Status() {
	skills := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(exampleSkills, "testdata/skills"),
		skillembed.WithToolName("mytool"),
	)

	dir, err := os.MkdirTemp("", "skills")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	report := func() {
		statuses, err := skills.Status(skillembed.InstallOptions{Dir: dir})
		if err != nil {
			log.Fatal(err)
		}
		for _, st := range statuses {
			fmt.Println(st.Skill.Name, st.State)
		}
	}

	report()
	if _, err := skills.Install(skillembed.InstallOptions{Dir: dir}); err != nil {
		log.Fatal(err)
	}
	report()

	// Output:
	// bare-skill missing
	// demo-skill missing
	// bare-skill up-to-date
	// demo-skill up-to-date
}

// A go/analysis driver parses the command line itself, and treats every
// non-flag argument as a package pattern. Intercept therefore runs before it.
func ExampleInstaller_Intercept() {
	skills := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(exampleSkills, "testdata/skills"),
		skillembed.WithToolName("mylint"),
	)

	// This is the whole of main, in order.
	skills.Intercept()
	// singlechecker.Main(mylint.Analyzer)
}

// Run is for a tool that already parses its own arguments. It reports errors
// instead of exiting, so the caller keeps control, and asking for help is not
// a failure.
func ExampleInstaller_Run() {
	skills := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(exampleSkills, "testdata/skills"),
		skillembed.WithToolName("mytool"),
		skillembed.WithOutput(os.Stdout),
	)

	if err := skills.Run(nil); err != nil && !errors.Is(err, skillembed.ErrHelp) {
		log.Fatal(err)
	}

	// Output:
	// Manage the skills embedded in mytool.
	//
	// Usage:
	//   mytool skill install   [flags] [skill...]
	//   mytool skill uninstall [flags] [skill...]
	//   mytool skill list      [flags] [skill...]
	//
	// Flags:
	//       --agent string   Target agent: {github-copilot|claude-code|cursor|codex|gemini|antigravity} (repeatable, or all) (default "github-copilot")
	//       --dir string     Install to a custom directory (overrides --agent and --scope)
	//   -f, --force          Overwrite existing skills
	//       --scope string   Installation scope: {project|user} (default "project")
	//       --dry-run        Report what would happen without writing
	//
	// Embedded skills:
	//   bare-skill
	//   demo-skill  A skill used by this module's tests. It is not meant to be installed.
}
