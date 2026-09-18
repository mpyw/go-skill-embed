package skillembed_test

import (
	"embed"
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
// instead of exiting, so the caller keeps control.
func ExampleInstaller_Run() {
	skills := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(exampleSkills, "testdata/skills"),
		skillembed.WithToolName("mytool"),
	)

	if len(os.Args) > 1 && os.Args[1] == "skill" {
		if err := skills.Run(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
}
