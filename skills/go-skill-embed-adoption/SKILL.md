---
name: go-skill-embed-adoption
description: Ship Agent Skills inside a Go binary with go-skill-embed, and give that binary a skill install command. Read this when adding the library to a tool, when choosing a front end, or when an install reports a state that is hard to act on. Covers the wiring for each front end, the four traps that are silent, and how to check the result.
license: MIT
---

# Adopting go-skill-embed

Written against **go-skill-embed v0.1**. Check the version first.

Read [the README](https://github.com/mpyw/go-skill-embed#readme) for the API.
This covers the decisions and the traps.

## Decide three things first

| Decision | Options | Default |
| --- | --- | --- |
| Front end | `Intercept`, `Run`, or an adapter module | none |
| Agents | every built-in agent, or a subset through `WithAgents` | every one |
| Command name | any word | `skill` |

The front end follows from what the tool already is.

| The tool is | Use |
| --- | --- |
| A `flag` package tool | `Intercept` before `flag.Parse` |
| A `go/analysis` driver | `Intercept` before `singlechecker.Main` |
| A [spf13/cobra](https://github.com/spf13/cobra) tool | `skillcobra.Command` |
| An [urfave/cli](https://github.com/urfave/cli) tool | `skillurfavev3.Command` or `skillurfavev2.Command` |
| Something that parses its own arguments | `Run`, which returns errors instead of exiting |

## Wire it

Skills live under `skills/<name>/SKILL.md`. That layout is the
[Agent Skills specification](https://agentskills.io/specification), and the
discovery reads no other.

```go
//go:embed skills
var skillsFS embed.FS

var skills = skillembed.NewInstaller(
	skillembed.MustSkillsFromFS(skillsFS, "skills"),
	skillembed.WithToolName("mytool"),
	skillembed.WithVersion(version),
)

func main() {
	skills.Intercept()
	flag.Parse()
}
```

## Four traps

Each is silent. None produces an error at the point where it goes wrong.

**A bare `//go:embed` drops a dotfile.** Every file whose name begins with `.`
or `_` is left out, and nothing says so. Write `all:skills` when a skill holds
one. That form keeps `.DS_Store` too, which the discovery then refuses by name.

**`embed.FS` carries no file mode.** A script arrives read-only. Any file
starting with `#!` is installed `0755`, and `WithExecutable` decides for a
script with no shebang.

**Installing writes into the skill's own `SKILL.md`.** Four `x-embedded-*` keys
go into the frontmatter, and they are what a later run compares against.
`WithMetadata(false)` removes them, and every installed copy then reads as
`foreign`.

**`Intercept` reads the first argument and nothing else.** `mytool skill
install` reaches it. `mytool -v skill install` does not. A tool's own help says
nothing about the command either, so print `UsageHint` from `flag.Usage` or
from an analyzer's `Doc`.

## What a state means

| State | The installed copy | What install does |
| --- | --- | --- |
| `missing` | Not there | Writes it |
| `up-to-date` | Matches | Skips it |
| `outdated` | Not what this binary would write | Overwrites it |
| `modified` | Edited after installing | Skips it, and reports `ErrNeedsForce` |
| `foreign` | Not something this tool wrote | Skips it, and reports `ErrNeedsForce` |

`outdated` covers a newer copy in the binary and a file that lost its
executable bit. `foreign` covers a hand-written skill, a directory with no
`SKILL.md`, and one that cannot be read at all.

A skipped skill does not stop the others. `Install` returns one result per
skill either way, and the error only says that something was left alone.

## Check the result

Run the real binary. The states are what the command reports, so a wiring
mistake shows up here and in no test.

```bash
go build -o /tmp/mytool . || exit 1

/tmp/mytool skill                      # the command, and the skills it carries
/tmp/mytool skill install --dry-run    # where they would go
/tmp/mytool skill install
/tmp/mytool skill list                 # every row should read up-to-date
```

> [!IMPORTANT]
> Project scope resolves against the project, not the working directory. The
> search walks up to the repository root, and the home directory is refused.
> Run the check from a subdirectory as well, and confirm the paths are the ones
> intended.

For a `go/analysis` driver, check that the driver still works.

```bash
/tmp/mylint ./...
go vet -vettool=/tmp/mylint ./...
```

## Traps when measuring

**A skill installed by an older build may read as `modified`.** The recorded
digest was computed by that build. Changing what the digest covers changes the
answer, and the user sees a file they never edited.

**`--dir` skips the agent table.** A check that passes with `--dir` says
nothing about where a real install lands.

**`--agent detected` depends on the machine.** It keeps the agents whose
directory is already there, and falls back to every agent when it finds none.
Two machines give two answers for the same command.
