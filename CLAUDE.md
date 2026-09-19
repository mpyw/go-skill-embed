# Notes for agents working on this repository

Run `./test_all.sh` before claiming anything passes. It covers every module.

| Section | What it holds |
| --- | --- |
| Where prose goes | Which file takes history, and which takes only the present |
| Rejected designs | What was tried, and the failure that ruled it out |
| Things that look wrong but are not | Deliberate oddities, so nobody "fixes" them |
| Known and left alone | Raised in review, and accepted |

## Where prose goes

| File | What belongs there |
| --- | --- |
| `README.md`, doc comments, source comments | The current state only. Brief |
| `CLAUDE.md` | History and rationale, including rejected designs |

A comment says why the code has its present shape, in the present tense. An
account of a past bug belongs here instead. A comment that only restates the
code is worth less than no comment.

## Rejected designs

### Frontmatter and digests

**A YAML library for the frontmatter.** `internal/manifest` edits bytes
instead. Install has to add four keys to a `SKILL.md` without disturbing the
rest of it. A parse and re-emit round trip rewrites quoting, key order and
block scalars, which produces a diff the skill author never wrote. It also
means the digest cannot be compared against the embedded original.

**Hashing the installed manifest as it stands.** The digest strips the four
injected keys first. Without that, an installed copy never hashes equal to the
skill it came from, and `up-to-date` can never be reported.

**A blank line between the added frontmatter and the body.** `manifest.With`
writes the closing `---` and then the body, with nothing between them, when it
creates a block that was not there. It reads slightly worse. It is also the
only way `Strip` can restore the file byte for byte, and without that a skill
whose `SKILL.md` had no frontmatter reads as `modified` the moment it is
installed. `ExampleInstaller_Status` is what caught it.

**Letting `Strip` remove a frontmatter block that turned out empty.** It could
not tell a block `With` had written from one the source already had, so a skill
whose manifest carried an empty block read as `modified` the moment it was
installed. `Normalize` removes an empty block on both sides instead, which
makes the two spellings of "no frontmatter" hash alike. `Strip` now only ever
removes the injected keys, and skips indented lines, because a block scalar may
hold a line that looks exactly like one.

**Hashing what the file system says about a mode.** An embedded file has no
mode at all, so the two sides of a comparison could never agree on one.
`ExecutableBitsMatch` applies the rule to the contents instead, which the
digest has already matched, so both sides reach the same answer. A lost
executable bit reads as `outdated` rather than `modified`, because the user did
not do it and repairing it should not need `--force`.

### Installing

**Removing the destination before renaming the staging directory in.** The
removal is not atomic either. One that failed half way left the old
installation destroyed and the new one unwritten, which is the opposite of what
the comment above it promised. The swap is two renames now, with the old
directory moved aside in between and moved back if the second one fails.

**Refusing the whole run when one destination needs `--force`.** One `foreign`
directory stopped every other skill from installing anywhere, and the refusal
came back as an error string with the results thrown away. `Uninstall` had
always reported the same situation per skill, as `ActionSkipped` with a
`Reason`. `Install` now does too, and the error wraps `ErrNeedsForce`. Both
verbs return results that mean something even when the error is not nil, and
every front end prints them before returning it.

**Letting a `Digest` error out of `inspect`.** A symlink inside an installed
skill, which is a thing a user does, made `Status` fail. `Install` and
`Uninstall` both call `Status` first, so `--force` failed too and the only way
out was `rm -rf`. One such directory took every other skill at that target with
it. An unreadable copy now reads as `foreign`: nothing can be said about what
is there, so nothing is claimed.

### Embedding

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

### The command line

**Registering `-skill.install` as an `analysis.Analyzer` flag.** It works, by
doing the install inside `flag.Value.Set` and calling `os.Exit`. It is also a
side effect in a place nobody expects one, and the flag name differs between
`singlechecker` and `multichecker`. `Intercept` guards on the literal first
argument instead. That is the one thing every driver leaves alone.

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

**Exporting half of the rendering.** `RenderCLIStatus` was the table alone, and
the description block above it lived in an unexported function, so `list` read
differently in the core and in the adapters. The exported renderer is now the
whole of what `list` prints, and it takes the skills from the statuses rather
than from the set.

### Module layout and scopes

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

Fourteen declarations carry a `//declscope:ignore qualify` of their own. Eleven
are the `With*` options, and three are `Run`, `Intercept` and `ErrHelp`. Both
groups are written by hand in a user's `main`, where a namespace in the name
costs more than it explains. Nothing else is suppressed.

**Exporting an accessor to get a field across a file.** declscope's boundary
rule only polices unexported declarations, so exporting something is the one
guaranteed way to silence it. Each such accessor was a permanent public promise
bought with a file split. The `Installer` fields that another file
reads carry a `//declscope:package` instead, which says the same
thing in the source and costs nothing outside the module. `CommandName`,
`ToolName`, `DefaultScope` and `AgentChoices` stay exported, because an adapter
in another module really does need them.

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

`scripts/checkdocs.py` compiles every Go block in the documents. A block is
hand written, so it drifts when a signature or a type changes, and twice it
did: once when `Install` gained a second return value, and once when
`InstallOptions.Agents` became `[]AgentSelector`. Both were found by a reader
rather than by the suite.

`scripts/regolden.py` rewrites the Output comments that hold help text. They
are goldens, and a flag or a default changing moves all four at once. Editing
them by hand invites a typo that reads as a real difference.

revive runs with `unused-receiver`, `exported` and `package-comments` on top of the standard
linters. The rename that produced the current names reached doc comments as
well as declarations, and left two of them starting with the wrong word.
Nothing else in the standard set looks at that.

`usageTrimLines` runs over every rendered block. A tabwriter pads the last
column, so a skill with no description printed trailing spaces. Nothing should
print those, and `gofmt` strips them from an `// Output:` comment, so an
example could not assert the help text until they were gone.

`_, _ = fmt.Fprintf(tw, ...)` appears only where the target is a `tabwriter`
over an in-memory buffer. errcheck's default exclusions cover `bytes.Buffer`
and `os.Stderr` but not `tabwriter`. The write that can actually fail is the
one to the real writer, and that one is checked.

**Resolving project scope against the working directory.** A linter is run from
a subdirectory as often as from the root, and a plain working directory put the
skills wherever that was. The search walks up to the repository root instead.

**Allowing the search to land on the home directory.** The user scope
directories are there, so a project installation would sit in front of every
other project, and `~/.claude/skills` in particular is exactly where `--scope
user` writes. Landing there is refused rather than allowed, because the
alternative is a user-wide install that nothing announced.

**Leaving the resolved project root to be inferred from a leaf path.** The
search depends on where the command was run, so a run from a subdirectory and
a run from the repository root can disagree, and each row's destination is the
only sign of it. Every project scope run names the root instead. Restricting
the markers to the selected agents was the other proposal and was rejected: it
makes `--agent claude-code` and `--agent all` resolve `.claude/skills` to
different directories in one repository.

**Writing the first-argument guard up as a limitation.** The README used to say
that a package directory named `skill` is hidden by it, which reads as
something taken away. Nothing is. `go` reads a bare name as an import path and
looks for it in the standard library, so `mylint skill` fails on a local
`skill/` package with or without the guard. Measured: a driver with no guard
answers `package skill is not in std`. The one real collision is a tool that
takes file names, and the note says so.

**Reaping the aside directory from a `defer`.** It looked like the one place
that covers every path, and it covered one it should not have. When the swap
fails and the restore fails too, the tree moved aside is the only copy left,
and the `defer` removed it. Measured against the previous shape: 3893 failed
writes, the old tree preserved in every one. The reap runs only after the swap
succeeds now, and a failed restore keeps the directory and names it in the
error.

**Letting `cliScope.Set` store an empty value.** `Targets` reads an empty
`Scope` as "use the default", so `--scope ""` was taken as a default rather
than as the mistake it is. The front end rejects it, because it is the only
place that knows the flag was given.

**A release per module.** Four pages saying the same thing, and a consumer
then has to work out which adapter goes with which core. Every module is
tagged with one version, and the release on the core tag says so.

**goreleaser.** There is nothing to build. A consumer reaches a library with
`go get`, and an attached binary would be a file nobody downloads.

## Known and left alone

An adversarial review raised these. They are recorded so the next reader does
not raise them again.

The threat model is the reason. Whoever can write to the skills directory is
almost always the person running the tool, and what breaks is their own
`~/.claude/skills`. Each of these needs the user to work against themselves,
and none of them corrupts anything they did not touch.

| | |
| --- | --- |
| Concurrent installs into one directory | Six of eight raced runs fail on the rename. No corruption, and the last one wins |
| A killed run leaves `.<name>.tmp-*` or `.<name>.old-*` behind | Nothing picks either up. The leading dot keeps them out of the agents' way. A failed restore leaves one on purpose, and names it |
| A container whose `HOME` is the repository root cannot install at project scope | The refusal cannot tell that shape from a real home directory. Weighed and kept: the refusal fails loudly and names the way out, and allowing it writes a project install into the user scope directory in silence. A devcontainer and a GitHub Actions runner both have a `HOME` above the repository. Neither reaches it. `skill install` is a developer's one-off rather than something CI runs |
| `AgentSelectorFor` does not round trip a name holding a comma or spaces | Selectors are split on commas and trimmed. It needs a custom agent through `WithAgents` |
| The adapters print no default for `--agent` | Each binds an empty slice, which cobra and urfave both read as "no default to show". The default used is the same in all four |
| A `SKILL.md` that is a fifo or `/dev/zero` | `inspect` reads it without a size or type guard, so it hangs or grows without bound |
| Extra `x-embedded-at` lines carry arbitrary text | `Strip` drops every injected key before hashing, so the digest cannot see them |
| The installed skill directory is 0700 | Inherited from `os.MkdirTemp`. Its subdirectories are 0755 |
| A file starting with `#![no_std]` becomes executable | The shebang test cannot tell it from a script. `WithExecutable` is the way out |
| A user's own `chmod +x` is reverted | The state is `outdated` either way, and install repairs it without asking |
| `uninstall` exits 0 where `install` exits 1 | Both skip a blocked destination and say so. Only install treats it as a failure |
| Names differing only by Unicode normalization are not caught | `strings.ToLower` is not the file system's equivalence relation. It catches the ASCII case, which is the one that happens |
| `--force` with `--dir` can remove an unrelated directory | It needs a skill whose name collides with something in that directory |
| `--agent ""` says "no agent selected" | The value is dropped as empty before anything can name it. `ErrNoAgentSelected` at least makes it matchable |
| `InstallOptions.Names` is not deduplicated | Naming a skill twice writes it twice |
| A BOM moves into the body | Only when `With` creates a frontmatter block that was not there |
| `quote` and `unquote` are asymmetric | A tool name holding a quote or a backslash never reads back, so the skill stays `foreign` |
