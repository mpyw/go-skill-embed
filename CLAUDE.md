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

## Things that look wrong but are not

The `replace` directives in the adapter modules point at `../`. Go ignores a
`replace` in a dependency, so consumers resolve the `require` line normally.
They are there so the repository builds before a tag exists.

`cliTrimLines` runs over every rendered block. A tabwriter pads the last
column, so a skill with no description printed trailing spaces. Nothing should
print those, and `gofmt` strips them from an `// Output:` comment, so an
example could not assert the help text until they were gone.

`_, _ = fmt.Fprintf(tw, ...)` appears only where the target is a `tabwriter`
over an in-memory buffer. errcheck's default exclusions cover `bytes.Buffer`
and `os.Stderr` but not `tabwriter`. The write that can actually fail is the
one to the real writer, and that one is checked.
