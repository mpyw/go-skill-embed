# go-skill-embed

[![CI](https://github.com/mpyw/go-skill-embed/actions/workflows/ci.yml/badge.svg)](https://github.com/mpyw/go-skill-embed/actions/workflows/ci.yml)

| Module | Reference |
| --- | --- |
| `github.com/mpyw/go-skill-embed` | [![Go Reference](https://pkg.go.dev/badge/github.com/mpyw/go-skill-embed.svg)](https://pkg.go.dev/github.com/mpyw/go-skill-embed) |
| `github.com/mpyw/go-skill-embed/skillcobra` | [![Go Reference](https://pkg.go.dev/badge/github.com/mpyw/go-skill-embed/skillcobra.svg)](https://pkg.go.dev/github.com/mpyw/go-skill-embed/skillcobra) |
| `github.com/mpyw/go-skill-embed/skillurfavev3` | [![Go Reference](https://pkg.go.dev/badge/github.com/mpyw/go-skill-embed/skillurfavev3.svg)](https://pkg.go.dev/github.com/mpyw/go-skill-embed/skillurfavev3) |
| `github.com/mpyw/go-skill-embed/skillurfavev2` | [![Go Reference](https://pkg.go.dev/badge/github.com/mpyw/go-skill-embed/skillurfavev2.svg)](https://pkg.go.dev/github.com/mpyw/go-skill-embed/skillurfavev2) |

Ship agent skills inside a Go binary, and give that binary a `skill install`
command.

`gh skill install` fetches skills from a GitHub repository. This library does
the same job from the other side. A tool carries its own skills in an
`embed.FS`, and writes them wherever the user's agent reads from. The flags
match `gh skill install`, so a user who knows that command already knows this
one.

## Install

```bash
go get github.com/mpyw/go-skill-embed
```

## Quick start

Skills live under `skills/<name>/SKILL.md`. That is the layout defined by the
[Agent Skills specification](https://agentskills.io/specification).

```go
package main

import (
	"embed"

	skillembed "github.com/mpyw/go-skill-embed"
)

//go:embed skills
var skillsFS embed.FS

var skills = skillembed.NewInstaller(
	skillembed.MustSkillsFromFS(skillsFS, "skills"),
	skillembed.WithToolName("mytool"),
	skillembed.WithVersion("v0.1.0"),
)

func main() {
	skills.Intercept()
	// the rest of your tool
}
```

> [!IMPORTANT]
> The two forms of `//go:embed` are not the same, and neither is always right.
>
> | Written | Kept | Dropped |
> | --- | --- | --- |
> | `//go:embed skills` | Ordinary files | Every name starting with `.` or `_`, silently |
> | `//go:embed all:skills` | Everything | Nothing, `.DS_Store` included |
>
> Write `all:` when a skill holds a file whose name starts with `.` or `_`.
> Write the bare form otherwise.

### Files an operating system leaves behind

`.DS_Store`, `Thumbs.db`, `desktop.ini` and `.localized` are handled from two
directions, so either `//go:embed` form is safe.

| Where the file is | What happens |
| --- | --- |
| Inside an embedded skill | `SkillsFromFS` refuses the skill and names the file |
| Beside an installed skill | Ignored. The skill still reads as `up-to-date` |

An embedded one was committed, and it ships to everyone. An installed skill
sits in a directory a user may open in a file browser, where such a file
appears on its own.

## Where skills go

The directories match `gh skill install`.

| Agent | Project scope | User scope |
| --- | --- | --- |
| `github-copilot` | `.agents/skills` | `~/.copilot/skills` |
| `claude-code` | `.claude/skills` | `~/.claude/skills` |
| `cursor` | `.agents/skills` | `~/.cursor/skills` |
| `codex` | `.agents/skills` | `~/.codex/skills` |
| `gemini` | `.agents/skills` | `~/.gemini/skills` |
| `antigravity` | `.agents/skills` | `~/.gemini/antigravity/skills` |

> [!IMPORTANT]
> Project scope resolves against the project, not against the working
> directory.
>
> | Where the command runs | Where the skills go |
> | --- | --- |
> | A directory already holding `.agents` or `.claude` | That directory |
> | Anywhere else inside a repository | The repository root |
> | Outside a repository | The working directory |
> | The home directory | Refused, with `ErrProjectIsHome` |
> | Outside the project, through a symbolic link | Refused, with `ErrProjectEscapes` |
>
> The search walks up from the working directory and stops at the repository
> root. The home directory holds the user scope directories, so a project
> installation there would sit in front of every other project. `--scope user`
> writes there on purpose, and `--dir` names any directory outright.
>
> A project install writes where the project's own tree says, and a path
> component is followed whatever the path reads as. A symbolic link at
> `.claude/skills`, or at any directory above it, therefore decides where the
> bytes land, and a link is something a repository can carry: git stores one as
> mode `120000`, so it survives a clone. The destination is resolved and
> refused when it leaves the project root. A link that stays inside the project
> is the project's own arrangement and is followed.
>
> The bound is project scope alone. User scope and `--dir` are the user naming
> a place, so a home directory moved with a link keeps working.
>
> Because the answer depends on where the command was run, every project scope
> run names it.
>
> ```
> Project root: /home/me/repo
> ```

Five of the six share `.agents/skills` at project scope. Selecting several of
them resolves to one directory. Each skill is written there once.

`--agent` also takes two words.

| Value | Constant | Meaning |
| --- | --- | --- |
| `detected` | `AgentSelectorDetected` | The agents whose directory is already there. The default |
| `all` | `AgentSelectorAll` | Every agent, present or not |

`--agent` is repeatable, and one value may be a comma separated list.
`--agent claude-code --agent cursor` and `--agent claude-code,cursor` name the
same two.

`InstallOptions.Agents` holds `AgentSelector` values. `AgentSelectorFor` names
one agent, so a caller reaches every form without writing a bare string.

```go
options := skillembed.InstallOptions{
	Agents: []skillembed.AgentSelector{
		skillembed.AgentSelectorFor(skillembed.AgentClaudeCode),
	},
}
```

`detected` falls back to `all` when it finds nothing, so a fresh repository
still gets its skills. In a repository that already holds `.claude`, only
Claude Code is written to. In a home directory it is the agents in use, rather
than six directories of which most are litter.

Claude Code moves its whole configuration with `CLAUDE_CONFIG_DIR`. User scope
follows that variable when it is set.

> [!NOTE]
> `gh skill install` defaults to `github-copilot`, and prompts for the agent
> when it can. A tool that embeds its skills is rarely able to prompt, and that
> default writes only `.agents/skills`, which Claude Code does not read.
> `WithDefaultAgents` restores the `gh` behaviour.

## The command

```
$ examplelint skill
Manage the agent skills embedded in examplelint.

Usage:
  examplelint skill install   [flags] [skill...]
  examplelint skill uninstall [flags] [skill...]
  examplelint skill list      [flags] [skill...]

Flags:
  -agent value
    	Target agent: {github-copilot|claude-code|cursor|codex|gemini|antigravity}, or all, or detected (repeatable) (default "detected")
  -dir string
    	Install to a custom directory (overrides -agent and -scope)
  -dry-run
    	Report what would happen without writing
  -f	Overwrite existing skills (shorthand)
  -force
    	Overwrite existing skills
  -scope value
    	Installation scope: {project|user} (default project)

Embedded skills:
  example-adoption  Stand-in skill for the singlechecker example. A real linter ships the skill that explains how to ...
```

That is `examples/singlechecker` in this repository, run for real.
`examplelint skill install -h` answers the same way, for that subcommand alone.
`list` and `uninstall` also answer to `ls` and `remove`, in every front end.

> [!NOTE]
> The frame is this library's. The flag block is the `flag` package's own, so
> it prints one dash and sits next to your tool's flags without looking
> foreign. Both `-agent` and `--agent` are accepted, as always with that
> package. `-f` and `-force` are two flags on one variable, which is why they
> print on two lines.
>
> The spf13/cobra and urfave/cli adapters print `--agent`, because that is
> what those frameworks print.

### Naming the command in your own help

> [!WARNING]
> Your tool's own help says nothing about the skill command. `Intercept` runs
> before your flags are even defined, and it cannot reach `flag.Usage` or an
> analyzer's `Doc`. Nobody finds the command unless you name it.

`UsageHint` is that line. It tracks the command name and the skill count, so it
cannot drift from what the command actually does.

```go
flag.Usage = func() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\n%s\n", skills.UsageHint())
}
```

```
Run "mytool skill" to install the 2 agent skills embedded in mytool.
```

A go/analysis driver builds its help from the analyzer, so the line goes in
`Analyzer.Doc`. `examples/singlechecker` does that.

`Usage` returns the full help text, for a tool that writes its own.

## What install does

Every installed `SKILL.md` gains four frontmatter keys.

```yaml
x-embedded-by: mytool
x-embedded-version: v0.1.0
x-embedded-at: "2026-09-18T16:09:53Z"
x-embedded-digest: "sha256:f6e4b378de0150621e981fd1b165edd04089cb3caac89b963308717ec71f8114"
```

The digest is what makes a second run safe. It covers the whole skill
directory. The manifest is hashed with these four keys removed, so an installed
copy and its embedded original hash the same.

| State | Meaning | What install does |
| --- | --- | --- |
| `missing` | Nothing is there | Writes it |
| `up-to-date` | The installed copy matches | Skips it |
| `outdated` | Not what this binary would write | Overwrites it |
| `modified` | The user edited it after installing | Skips it, and reports `ErrNeedsForce` |
| `foreign` | Not something this tool wrote | Skips it, and reports `ErrNeedsForce` |

A skipped skill does not stop the others. `Install` writes everything it can,
returns one `InstallResult` per skill either way, and returns an error wrapping
`ErrNeedsForce` when it left anything alone. The results are meaningful even
when the error is not.

```go
results, err := skills.Install(ctx, o)
report(results)
if errors.Is(err, skillembed.ErrNeedsForce) {
	// tell the user to re-run with --force
}
```

`foreign` is wider than "another tool put it there". It also covers a
hand-written skill, a directory with no `SKILL.md`, one whose `SKILL.md` has no
`x-embedded-*` keys, and one this tool cannot read at all, such as a directory
holding a symlink. Nothing can be said about any of them, so nothing is
claimed, and `--force` remains the way through.

> [!WARNING]
> `WithMetadata(false)` turns the four keys off. Install can then no longer
> tell an outdated copy from an edited one. Every existing directory reads as
> `foreign`.

## Frameworks

The core module has no dependencies beyond the standard library. Each adapter
is a module of its own, so embedding skills never pulls spf13/cobra into your
linter.

| Framework | Module | How |
| --- | --- | --- |
| stdlib `flag` | core | `skills.Intercept()` |
| `singlechecker`, `multichecker`, `unitchecker` | core | `skills.Intercept()` |
| [spf13/cobra](https://github.com/spf13/cobra) | `github.com/mpyw/go-skill-embed/skillcobra` | `root.AddCommand(skillcobra.Command(skills))` |
| [urfave/cli](https://github.com/urfave/cli) v3 | `github.com/mpyw/go-skill-embed/skillurfavev3` | `skillurfavev3.Command(skills)` |
| [urfave/cli](https://github.com/urfave/cli) v2 | `github.com/mpyw/go-skill-embed/skillurfavev2` | `skillurfavev2.Command(skills)` |

> [!IMPORTANT]
> `Intercept` and the adapters take the first argument. Check that `skill` does
> not already mean something in your tool.
>
> | Your tool | Can `skill` already mean something else? |
> | --- | --- |
> | spf13/cobra or urfave/cli | No. The first argument is a subcommand |
> | A `flag` tool with subcommands | No, for the same reason |
> | A go/analysis driver | No. The first argument is a Go package pattern, and `skill` is not one |
> | A tool that takes file names | **Yes**, if a file is called `skill` |
>
> `go` reads a bare name as an import path, not a directory, so a local package
> is written `./skill` with or without this library.
>
> For the last row, reach the file as `./skill`, or rename the subcommand with
> `WithCommandName`. A `flag` tool is worth giving subcommands anyway, and then
> the row does not apply.

> [!NOTE]
> One thing the four front ends cannot agree on is a flag written after a
> positional argument. `mytool skill install demo --dry-run` works under
> spf13/cobra and urfave/cli v3. The `flag` package and urfave/cli v2 read it
> as a second skill name. That is each framework's own parser, not this
> library. Writing flags before names works everywhere.

### stdlib flag

`Intercept` goes before `flag.Parse`. The skill command is not a flag, so
`flag.Parse` has nothing to do with it.

```go
func main() {
	skills.Intercept()
	flag.Parse()
	// the rest of your tool
}
```

> [!IMPORTANT]
> `Intercept` looks at the first argument and nothing else.
> `mytool skill install` reaches it. `mytool -v skill install` does not.
> Put your own flags after the subcommand, or before a normal run.

### go/analysis drivers

`singlechecker`, `multichecker` and `unitchecker` parse the command line
themselves. Every non-flag argument is a package pattern to them. No hook
exists after the driver starts, so running before it is the only option.

```go
func main() {
	skills.Intercept()
	singlechecker.Main(mylint.Analyzer)
}
```

A `go vet -vettool=` run passes `-flags` or a config file path, so it is never
affected. `examples/singlechecker` is a working driver that does this, with
tests that run the real binary both ways.

### spf13/cobra and urfave/cli

```go
root.AddCommand(skillcobra.Command(skills))
```

```go
app := &cli.Command{
	Name:     "mytool",
	Commands: []*cli.Command{skillurfavev3.Command(skills)},
}
```

## Options

| Option | Default | |
| --- | --- | --- |
| `WithToolName` | The binary's name | Recorded in `x-embedded-by` |
| `WithVersion` | Empty | Recorded in `x-embedded-version` |
| `WithCommandName` | `skill` | The subcommand `Run` and `Intercept` answer to |
| `WithAgents` | All six | Restricts what `--agent` accepts, and which directories the project root search looks for |
| `WithDefaultAgents` | `detected` | Used when `--agent` is absent |
| `WithDefaultScope` | `project` | Used when `--scope` is absent |
| `WithProjectRoot` | The searched project root | What project scope resolves against |
| `WithMetadata` | On | Writes the four `x-embedded-*` keys |
| `WithExecutable` | Shebang test | Decides which files become executable |
| `WithOutput` | `os.Stdout` | Where `Run` writes the report and the help |
| `WithErrorOutput` | `os.Stderr` | Where `Run` writes a complaint and the usage |

> [!CAUTION]
> `embed.FS` does not carry file modes. Every embedded file arrives read-only.
> A script installed without repair cannot be run by the agent.
> The default marks any file starting with `#!` as executable.
> Pass `WithExecutable` when your scripts have no shebang.

## Using it as a library

`Run` never calls `os.Exit`, so a driver keeps control.

`Status` reports without changing anything. `Install` and `Uninstall` return
one `InstallResult` per skill per destination. All three take a context and
stop between skills when it is cancelled.

```go
results, err := skills.Install(ctx, skillembed.InstallOptions{
	Agents: []skillembed.AgentSelector{
		skillembed.AgentSelectorFor(skillembed.AgentClaudeCode),
	},
	Scope: skillembed.ScopeUser,
})
```

`RenderCLIResults` and `RenderCLIStatus` turn those values into the text the
built-in command prints. A front end that calls them reports the same way.

Every error a user's own input can cause wraps a sentinel, so a front end can
tell a mistyped flag from a disk that is full.

| Error | Cause |
| --- | --- |
| `ErrUnknownAgent` | `--agent` named no agent |
| `ErrUnknownScope` | `--scope` was neither `project` nor `user` |
| `ErrUnknownSkill` | A named skill is not embedded |
| `ErrNoAgentSelected` | The values resolved to nothing |
| `ErrNeedsForce` | A destination was left alone. `ForceRequiredError` names them |
| `ErrProjectIsHome` | Project scope resolved to the home directory |
| `ErrProjectEscapes` | A project scope destination is outside the project root |

## The skill for this library

`skills/go-skill-embed-adoption/SKILL.md` covers adopting the library: which
front end to choose, the four traps that are silent, and how to check the
result. Install it into a repository that is about to embed skills.

```bash
gh skill install mpyw/go-skill-embed go-skill-embed-adoption --agent claude-code
```

## Development

Tools are pinned in `mise.toml`.

```bash
mise install
./test_all.sh
```

`scripts/regolden.py` rewrites the help text that the examples assert. Run it
after changing a flag or a default.

A release is cut by the **Tag and Release** workflow. It takes a version, tags
every module with it, and publishes one release on the core tag that stands
for all of them. `scripts/modules.sh` is where the module list comes from, so
a new adapter needs no change to the workflows.

Declaration scopes are enforced by [declscope](https://github.com/mpyw/declscope),
at `qualify: ondemand` with `exported: true`. The settings are in
`.declscope.yaml`, and its adoption skill is installed at
`.claude/skills/declscope-adoption`.

## License

MIT
