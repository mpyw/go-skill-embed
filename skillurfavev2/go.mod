module github.com/mpyw/go-skill-embed/skillurfavev2

go 1.24

toolchain go1.27.1

require (
	github.com/mpyw/go-skill-embed v0.2.1
	github.com/urfave/cli/v2 v2.27.7
)

require (
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
)

// The adapters live in their own modules so that a linter embedding skills
// never takes on cobra, or urfave/cli, through the core.
replace github.com/mpyw/go-skill-embed => ../
