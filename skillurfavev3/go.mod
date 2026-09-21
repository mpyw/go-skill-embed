module github.com/mpyw/go-skill-embed/skillurfavev3

go 1.24

toolchain go1.27.1

require (
	github.com/mpyw/go-skill-embed v0.2.1
	github.com/urfave/cli/v3 v3.12.0
)

// The adapters live in their own modules so that a linter embedding skills
// never takes on cobra, or urfave/cli, through the core.
replace github.com/mpyw/go-skill-embed => ../
