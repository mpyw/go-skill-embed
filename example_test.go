package skillembed_test

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	skillembed "github.com/mpyw/go-skill-embed"
)

// These skills hold no file whose name begins with a dot or an underscore, so
// the bare form is enough. Write all:testdata/skills when one does, since the
// bare form drops them without a word.
//
//go:embed testdata/skills
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
	ctx := context.Background()
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

	results, err := skills.Install(ctx, skillembed.InstallOptions{
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
	results, err = skills.Install(ctx, skillembed.InstallOptions{
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
	ctx := context.Background()
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
		statuses, err := skills.Status(ctx, skillembed.InstallOptions{Dir: dir})
		if err != nil {
			log.Fatal(err)
		}
		for _, st := range statuses {
			fmt.Println(st.Skill.Name, st.State)
		}
	}

	report()
	if _, err := skills.Install(ctx, skillembed.InstallOptions{Dir: dir}); err != nil {
		log.Fatal(err)
	}
	report()

	// Output:
	// bare-skill missing
	// demo-skill missing
	// bare-skill up-to-date
	// demo-skill up-to-date
}

// Intercept goes before flag.Parse, because the skill command is not a flag
// and has to be the first argument.
func ExampleInstaller_Intercept() {
	skills := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(exampleSkills, "testdata/skills"),
		skillembed.WithToolName("mytool"),
	)

	verbose := flag.Bool("v", false, "print what is happening")

	// Your own help says nothing about the skill command unless you say it.
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n%s\n", skills.UsageHint())
	}

	// `mytool skill install` is handled here and never returns.
	// `mytool -v ./...` falls through to the tool itself.
	skills.Intercept()
	flag.Parse()

	fmt.Println(*verbose, flag.Args())
}

// A go/analysis driver reads every non-flag argument as a package pattern, so
// there is no point after it starts at which a subcommand is still visible.
func ExampleInstaller_Intercept_analysisDriver() {
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
	ctx := context.Background()
	skills := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(exampleSkills, "testdata/skills"),
		skillembed.WithToolName("mytool"),
		skillembed.WithOutput(os.Stdout),
	)

	if err := skills.Run(ctx, nil); err != nil && !errors.Is(err, skillembed.ErrHelp) {
		log.Fatal(err)
	}

	// Output:
	// Manage the agent skills embedded in mytool.
	//
	// Usage:
	//   mytool skill install   [flags] [skill...]
	//   mytool skill uninstall [flags] [skill...]
	//   mytool skill list      [flags] [skill...]
	//
	// Flags:
	//   -agent value
	//     	Target agent: {github-copilot|claude-code|cursor|codex|gemini|antigravity}, or all, or detected (repeatable) (default "detected")
	//   -dir string
	//     	Install to a custom directory (overrides -agent and -scope)
	//   -dry-run
	//     	Report what would happen without writing
	//   -f	Overwrite existing skills (shorthand)
	//   -force
	//     	Overwrite existing skills
	//   -scope value
	//     	Installation scope: {project|user} (default project)
	//
	// Embedded skills:
	//   bare-skill
	//   demo-skill  A skill used by this module's tests. It is not meant to be installed.
}
