// Package skillembed ships agent skills inside a Go binary.
//
// It is the reverse of `gh skill install`. A tool carries its own skills in an
// embed.FS, and writes them into the directory the user's agent reads from.
// The agent directories and the flag set are taken from `gh skill install`, so
// a user who knows that command already knows this one.
//
// Skills live under skills/<name>/SKILL.md, which is the layout defined by the
// Agent Skills specification (https://agentskills.io/specification). Embed the
// directory with the all: prefix. A bare //go:embed drops every file whose
// name begins with a dot or an underscore, and says nothing about it.
//
//	//go:embed all:skills
//	var skillsFS embed.FS
//
//	var skills = skillembed.NewInstaller(
//		skillembed.MustSkillsFromFS(skillsFS, "skills"),
//		skillembed.WithToolName("mytool"),
//		skillembed.WithVersion("v0.1.0"),
//	)
//
// Installer.Intercept gives a tool the skill subcommand in one line. It works
// for a go/analysis driver too, where no hook exists once the driver starts.
// Installer.Run is for a tool that parses its own arguments. The skillcobra,
// skillurfavev3 and skillurfavev2 modules wire the same commands into those
// frameworks.
package skillembed
