# Notes for agents working on this repository

Run `./test_all.sh` before claiming anything passes. It covers every module.

## Designs that were tried and rejected

**A YAML library for the frontmatter.** `internal/manifest` edits bytes instead.
Install has to add four keys to a `SKILL.md` without disturbing the rest of it.
A parse and re-emit round trip rewrites quoting, key order and block scalars,
which produces a diff the skill author never wrote. It also means the digest
cannot be compared against the embedded original.

**Hashing the installed manifest as it stands.** The digest strips the four
injected keys first. Without that, an installed copy never hashes equal to the
skill it came from, and `up-to-date` can never be reported.

**Registering `-skill.install` as an `analysis.Analyzer` flag.** It works, by
doing the install inside `flag.Value.Set` and calling `os.Exit`. It is also a
side effect in a place nobody expects one, and the flag name differs between
`singlechecker` and `multichecker`. `Intercept` guards on the literal first
argument instead. That is the one thing every driver leaves alone.

**Putting the adapters in the core module.** This library exists to be embedded
in linters. A `tool` directive or a cobra import in the root `go.mod` reaches
every consumer's module graph. The adapters are separate modules, and declscope
is pinned in `mise.toml` rather than in `go.mod`, for the same reason.

**Reaching `exported: true` by renaming everything the tool suggested.** The
first measurement was 47 exported diagnostics. Most of them were the file
layout, not the names. Splitting `scope.go`, `meta.go`, `state.go` and
`action.go` out, and renaming `installer.go` to `install.go`, absorbed 34 of
them and left the names better than they were. `InstallOptions` and
`InstallResult` say more than `Options` and `Result` did.

**Reaching it with `//declscope:core` on the API files.** That is the remedy
the adoption skill names for a file that is the package's API. Every file in
the root package is, so it would leave declscope checking nothing there.

Thirteen declarations carry a `//declscope:ignore qualify` of their own. Ten
are the `With*` options, and three are `Run`, `Intercept` and `ErrHelp`. Both
groups are written by hand in a user's `main`, where a namespace in the name
costs more than it explains. Nothing else is suppressed.

**A blank line between the added frontmatter and the body.** `manifest.With`
writes the closing `---` and then the body, with nothing between them, when it
creates a block that was not there. It reads slightly worse. It is also the
only way `Strip` can restore the file byte for byte, and without that a skill
whose `SKILL.md` had no frontmatter reads as `modified` the moment it is
installed. `ExampleInstaller_Status` is what caught it.

**Taking `gh skill install`'s default agent along with its flags.** It is
`github-copilot`, and `gh` prompts for the agent whenever it can. A tool that
embeds its skills is rarely able to prompt, and that default writes only
`.agents/skills`, which Claude Code does not read. A Claude Code user running
`mylint skill install` would have seen a success and got nothing. The default
is `detected`, which falls back to `all`.

`gh` does no agent detection at all. Measured: with `.claude/skills` already in
the working directory, `gh skill install --from-local` still wrote
`.agents/skills`. The help's "auto-discovered" is about finding skills in a
repository, not about finding agents.

**Telling everyone to write `//go:embed all:skills`.** The bare form drops
every name starting with a dot or an underscore and says nothing, which is why
that advice was there. `all:` keeps `.DS_Store` too, and shipping one to every
user is worse than dropping a file the author would notice missing. Neither
form is right on its own, so the README states both and the library covers the
case that cannot be caught by reading.

Junk files are handled from two directions on purpose. `SkillsFromFS` refuses
one, because an embedded `.DS_Store` was committed and ships. `skillfs.Digest`
and `skillfs.Write` skip them, because an installed skill sits where a file
browser can reach it. Before that, opening `~/.claude/skills/<name>` in Finder
made the skill read as `modified` and install refuse without `--force`.

## Things that look wrong but are not

The `replace` directives in the adapter modules point at `../`. Go ignores a
`replace` in a dependency, so consumers resolve the `require` line normally.
They are there so the repository builds before a tag exists.

**Writing the flag block out by hand to match `gh skill install`.** It printed
`--agent` and `-f, --force`, which read well and were a lie. `-f` and `-force`
are two flags on one variable, the `flag` package has no short and long forms,
and a tool built on it prints one dash everywhere else. `usageFor` now takes
the FlagSet that parses the arguments and calls `PrintDefaults` on it. The
frame around the block is still this library's, because that is the part the
`flag` package cannot give.

The adapters print `--agent`, since cobra and urfave/cli do. Each front end
should read as if the host framework wrote it.

`Intercept` cannot make the skill command discoverable on its own. It runs
before the tool's own flags exist, and it can reach neither `flag.Usage` nor an
analyzer's `Doc`. `UsageHint` is a line the tool prints itself, and
`examples/singlechecker` shows where it goes.

`scripts/regolden.py` rewrites the Output comments that hold help text. They
are goldens, and a flag or a default changing moves all four at once. Editing
them by hand invites a typo that reads as a real difference.

revive runs with `exported` and `package-comments` on top of the standard
linters. The rename that produced the current names reached doc comments as
well as declarations, and left two of them starting with the wrong word.
Nothing else in the standard set looks at that.

`cliTrimLines` runs over every rendered block. A tabwriter pads the last
column, so a skill with no description printed trailing spaces. Nothing should
print those, and `gofmt` strips them from an `// Output:` comment, so an
example could not assert the help text until they were gone.

`_, _ = fmt.Fprintf(tw, ...)` appears only where the target is a `tabwriter`
over an in-memory buffer. errcheck's default exclusions cover `bytes.Buffer`
and `os.Stderr` but not `tabwriter`. The write that can actually fail is the
one to the real writer, and that one is checked.
