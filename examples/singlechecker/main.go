// Command examplelint shows how a go/analysis driver ships its own skills.
//
// singlechecker, multichecker and unitchecker all treat every non-flag
// argument as a package pattern, and they parse the command line themselves.
// There is therefore no point after the driver starts at which a subcommand
// could still be recognised, so Intercept runs first.
//
//	examplelint skill install --agent claude-code --scope user
//	examplelint ./...
package main

import (
	"embed"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"

	skillembed "github.com/mpyw/go-skill-embed"
)

// all: keeps files whose names begin with a dot or an underscore, which a bare
// //go:embed would drop without saying so.
//
//go:embed all:skills
var skillsFS embed.FS

var skills = skillembed.NewInstaller(
	skillembed.MustSkillsFromFS(skillsFS, "skills"),
	skillembed.WithToolName("examplelint"),
	skillembed.WithVersion("v0.1.0"),
)

// A driver's own -h is built from the analyzer, and knows nothing about the
// skill command. UsageHint is the one line that makes it discoverable.
var analyzer = &analysis.Analyzer{
	Name: "examplelint",
	Doc:  "an analyzer that reports nothing, so that the example stays about the skills\n\n" + skills.UsageHint(),
	Run:  func(*analysis.Pass) (any, error) { return nil, nil },
}

func main() {
	// Before the driver. The guard is the literal first argument, so a
	// `go vet -vettool=` invocation, which passes -flags or a config file
	// path, never reaches it.
	skills.Intercept()

	singlechecker.Main(analyzer)
}
