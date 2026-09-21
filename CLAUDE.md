# Notes for agents working on this repository

Run `./test_all.sh` before claiming anything passes. It covers every module.

| Section | What it holds |
| --- | --- |
| Where prose goes | Which file takes history, and which takes only the present |
| Rejected designs | What was tried, and the failure that ruled it out |
| Platforms | What differs by platform, and what CI does about it |
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

**Comparing the rule against a mode on a file system that has no modes.**
Windows builds a regular file's mode from the read-only attribute alone, so
`Mode()&0o111` is zero for a regular file and `HasShebang` returning true could
never agree with it. A skill holding a shebang file read as `outdated` on every
run, `install` rewrote it every time, and `--dry-run` reported an update that
was not one. `modesMatter` is a build-tagged constant and
`ExecutableBitsMatch` answers true at once where it is false, which leaves the
digest to decide alone. Issue #7 has it, from a reader porting the library to
Rust. `ExampleInstaller_Status` covers it: the demo skill's `scripts/run.sh`
has a shebang, so the example prints `outdated` on Windows without the fix.

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

**Bounding project scope with a string comparison.** `strings.HasPrefix` on
the two paths cannot see a symbolic link, which is the only thing that moves a
destination. Both sides are resolved with `filepath.EvalSymlinks` first, as far
as they exist, because a skills directory usually does not yet.

**Refusing every symbolic link on the way to a project destination.** A
repository that keeps its skills elsewhere in its own tree and links to them is
doing nothing wrong. What matters is where the link lands, not that there is
one.

**Bounding user scope the same way.** `~/.claude` moved onto another disk with
a link is a normal arrangement, and the user made it. The bound exists because
a project root is whatever was cloned, which is someone else's choice.

**Exempting a custom `Agent` whose `ProjectDir` climbs out with `..`.** It
could be read as the embedding tool's own choice, the way `--dir` is. It is
refused with everything else, because a project install landing outside the
project is the thing being stopped and who wrote the path does not change
that. `WithProjectRoot` and `--dir` are how a tool aims elsewhere. The
refusal's text says "outside the project root" rather than naming a link, so
this reader is not sent looking for one.

**Re-checking at write time.** The check runs in `Targets`, so a link created
between there and the rename is not caught. The case it is for is a link
committed into a repository, which is there before the run starts.

### Sweeping

**Giving `--force` a part in it.** It was the shape of three rounds of
findings before the premise itself was questioned. `--force` means "something
is in the way of what I am installing; overwrite it". Nothing is being
installed over an orphan, so there is no conflict, and a flag that let the
sweep take a directory whose contents had changed was authorising a deletion
nothing required. Worse, the tool asks for `--force` whenever anything reads
as modified, so following its own advice destroyed a user's edited fork of a
shipped skill. Contents that no longer match are the evidence the directory is
not this tool's any more; the sweep just stops there, and the flag keeps one
meaning.

**Finding them from a manifest of what was installed.** It is what
`gh skill install` and fslc both keep, and it is the thing this library does
not have: state lives in each installed `SKILL.md`, which is why a half
finished run is always recoverable and why a version mismatch can never orphan
a payload. The sweep reads the same stamps instead, so nothing new has to be
kept in step.

**Sweeping on `install <name>`.** Only a run over the whole set knows what is
missing from it. A named run is about those names, and removing a skill it was
never asked about on the way past is not something the user could have
predicted. Naming an orphan does reach it, because `list` prints one and
refusing a name the tool just printed is the worse answer.

**Matching an orphan to an installed skill by folded name.** `strings.ToLower`
is not the file system's equivalence relation, which is recorded below.
`os.SameFile` asks the file system instead, and a skill renamed to another
spelling of itself is one directory whichever way it is spelled. Left alone,
the sweep deleted what the same run had just written and exited 0.

**Sweeping the `.tmp-` and `.old-` directories `skillfs.Write` leaves.** The
rescue copy is a whole installation, stamp and digest and all, so it reads as
an orphan exactly. It is also the only copy left, and the error that abandoned
it names it for the user to recover. Every dot-prefixed entry is passed over.

**Testing the `x-embedded-by` check with a hand-made fixture.** A directory
carrying another name in `x-embedded-by` and a digest that does not match its
own contents is turned away by the digest, whether or not the name is ever
read, so the test passed with the check deleted. The fixture is a real
installation made by a second `Installer` with another `WithToolName`, where
the stamp is well formed and the digest checks out, and the name is the only
thing left between it and the sweep.

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

**Declaring `modesMatter` shared with `//declscope:package`.** A build tag
cannot apply to part of a file, so the constant lives in two files of its own,
and each one is a namespace. Two rules fire on that: `skillfs.go` reaching the
constant is a boundary crossing, and `qualify: ondemand` wakes up the moment a
package holds a second namespace, which asks for `modesUnixModesMatter` and
for a rename of every other name in the package as well. Measured:
`//declscope:package` clears the first and leaves the second.
`//declscope:namespace skillfs` clears both, and says what is true, which is
that the constant is `skillfs.go`'s.

**Exporting an accessor to get a field across a file.** declscope's boundary
rule only polices unexported declarations, so exporting something is the one
guaranteed way to silence it. Each such accessor was a permanent public promise
bought with a file split. The `Installer` fields that another file
reads carry a `//declscope:package` instead, which says the same
thing in the source and costs nothing outside the module. `CommandName`,
`ToolName`, `DefaultScope` and `AgentChoices` stay exported, because an adapter
in another module really does need them.

## Platforms

**Rejected: Linux alone in CI.** Every platform-dependent line in this
repository carried a comment about its platform behaviour and had never been
run anywhere but Linux: the two-rename swap, the executable bit,
`filepath.EvalSymlinks`, `filepath.Rel` across volumes,
`filepath.ListSeparator`. The `check` job is a three-platform matrix now, with
`fail-fast` off, because a failure on one platform is the interesting case and
the answer is usually whether the other two agree. `coverage` stays on Linux:
which lines run is the same question everywhere.

**Rejected: running `test_all.sh` only on Linux and the tests alone
elsewhere.** It would have kept `checkdocs` off Windows at the cost of one
flag, and it would also keep golangci-lint and declscope off the Windows
build, which is where the platform stubs are. The three Windows
`checkdocs.py` fixes were smaller than the flag, and none of them made the
script worse. golangci-lint, declscope and checkdocs all run on Windows, so
nothing had to be held back.

Four things had to change before the matrix could report anything but its own
setup.

| | |
| --- | --- |
| `.gitattributes` | Git for Windows installs with `core.autocrlf=true`, and a shebang line ending in a carriage return is not a shebang. A skill tree is exempted with `-text`, because git normalizing one silently changes a digest |
| `testenv.SetHome` | `os.UserHomeDir` reads `USERPROFILE` on Windows. A test setting `HOME` alone passes on Linux and reaches the developer's real home directory on Windows, which is where a user scope install would land. It clears `CLAUDE_CONFIG_DIR` too, which overrides the home-derived path outright: with that set, three adapter tests failed |
| `testenv.RequireSymlink` | Windows gates a symbolic link behind a privilege an account may or may not hold, so the helper asks by making one. A refusal reaches stderr as well as the test, because `go test` prints a skip only under `-v` and a run that quietly dropped every symbolic link test reads exactly like one that passed them |
| `checkdocs.py` | `go build -o /dev/null`, a `replace` path spelled with backslashes and unquoted, and the locale's encoding. None of them is about the documents |

What the first three-platform run found, beyond the bug it was added for:

| | |
| --- | --- |
| macOS | `checkdocs` builds its scaffold in a temporary directory, and a mise shim outside the repository has no version to resolve: "No version is set for shim: go". It asks `go env GOROOT` from inside the repository and runs that binary now |
| Windows | `TestWriteRestoresTheExecutableBit` read a mode back off the disk, which `modesMatter` had not reached |
| Windows | `examples/singlechecker` built `examplelint` under that name, and Windows will not exec a file without the extension |

`skipWithoutExecutableBits` in `install_test.go` spells the platform out with
`runtime.GOOS`, rather than reading `skillfs.modesMatter`. The constant is
unexported and the test is in another package, and exporting it to reach it
would be a promise made for a test. The duplication fails loudly: forcing
`modesMatter` to false on macOS makes both guarded tests fail rather than pass
vacuously.

Two tests assert nothing on Windows and say so, rather than passing quietly.
`TestWriteSucceedsWhenTheAsideTreeCannotBeRemoved` makes a directory
unremovable with a mode, which there sets the read-only attribute that
`os.RemoveAll` clears on its own, so the path it exists for is never entered.
`TestExecutableBitsMatchWithoutModes` is the other way round: it is the only
test that states what `ExecutableBitsMatch` promises where there is no bit, so
it skips everywhere else.

`TestWithinAcrossVolumes` is the branch the matrix was supposed to reach and
did not. `filepath.Rel` answers with an error across two volumes and `Within`
reads that as outside, and every other test took both paths from one root. The
runner hands over two volumes for free: the workspace is on `D:` and
`t.TempDir()` is on `C:`.

Two of the reasons first given for `.gitattributes` did not survive being
measured, and both are recorded because the file is easy to read as doing more
than it does. The `// Output:` comments were said to need it: they do not,
because `go/ast` strips a trailing carriage return before an example is
compared, and a checkout converted to CRLF passes every test. CI was said to
need it: it does not. rust-skill-embed runs the same three-platform matrix over
a bash `test_all.sh` with no `.gitattributes` at all, and its Windows job is
green, so the runner is not rewriting anything. What is left is a Windows
contributor's own clone, where `core.autocrlf` is on by default. Measured
there: `env: bash\r: No such file or directory`.

**Rejected: letting `RequireSymlink` skip on CI as it does locally.**
`go test` prints a skip only under `-v`, and `internal/projectroot`'s line is
`ok ... 0.164s` whether seven tests ran or seven skipped. A runner image that
stopped granting the privilege would drop the whole of `Within`, which guards
the one threat this repository does not leave alone, behind a green tick. It
skips for a developer, who should be able to run the suite without the
privilege, and fails when `GITHUB_ACTIONS` is set.

## Things that look wrong but are not

The `replace` directives in the adapter modules point at `../`. Go ignores a
`replace` in a dependency, so consumers resolve the `require` line normally.
They are there so the repository builds before a tag exists.

That `require` line is the one thing in the repository nothing checks by
building. The `replace` satisfies every local build and every CI job, so the
version beside it can say anything. v0.2.0 shipped three adapters still asking
for the core at v0.1.0, and it was invisible for exactly that reason.
Measured: `go get github.com/mpyw/go-skill-embed/skillcobra@v0.2.0` into an
empty module resolves `github.com/mpyw/go-skill-embed v0.1.0`, which compiles,
and which has neither the project root guard nor the orphan sweep in it. The
Tag workflow reads the line now and refuses a version the modules do not ask
for, which also means the bump has to be committed before the tag is cut.

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

"Almost always" is the project root. Its contents come from whoever the
repository was cloned from, so a symbolic link committed at `.claude/skills` or
above it aims a project install, and a later `--force` removal, anywhere on the
disk. That one is not on the list below: `projectroot.Within` refuses it.

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
| A Unix binary installing onto a mount that synthesizes modes reads as `outdated` forever | `modesMatter` is decided by `GOOS`, and a `GOOS=linux` binary writing to vfat, exfat, SMB without unix extensions, or WSL2's `/mnt/c` gets one mode for every file. Whichever way it goes, some file disagrees with the rule. The symptoms are issue #7's and so is the cost: a rewrite of a byte-identical tree on every run. A per-destination probe was the other proposal and cannot tell that mount from a user who ran `chmod +x` on one file, which is the case the check exists for |
| `uninstall` exits 0 where `install` exits 1 | Both skip a blocked destination and say so. Only install treats it as a failure |
| Names differing only by Unicode normalization are not caught | `strings.ToLower` is not the file system's equivalence relation. It catches the ASCII case, which is the one that happens |
| `--force` with `--dir` can remove an unrelated directory | It needs a skill whose name collides with something in that directory |
| `--agent ""` says "no agent selected" | The value is dropped as empty before anything can name it. `ErrNoAgentSelected` at least makes it matchable |
| `InstallOptions.Names` is not deduplicated | Naming a skill twice writes it twice |
| A BOM moves into the body | Only when `With` creates a frontmatter block that was not there |
| `quote` and `unquote` are asymmetric | A tool name holding a quote or a backslash never reads back, so the skill stays `foreign` |
