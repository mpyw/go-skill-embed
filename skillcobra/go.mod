module github.com/mpyw/go-skill-embed/skillcobra

go 1.24

toolchain go1.27.1

require (
	github.com/mpyw/go-skill-embed v0.1.0
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)

// The adapters live in their own modules so that a linter embedding skills
// never takes on cobra, or urfave/cli, through the core.
replace github.com/mpyw/go-skill-embed => ../
