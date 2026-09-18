# go-skill-embed

Ship agent skills inside your Go binary, and give it a `skill install` command.

`gh skill install` fetches skills from a GitHub repository. This library does
the same job from the other side. Your tool carries its own skills in an
`embed.FS`, and writes them wherever the user's agent reads from.

The flags match `gh skill install`, so a user who knows that command already
knows yours.

## Install

```bash
go get github.com/mpyw/go-skill-embed
```

## Quick start

Put your skills under `skills/<name>/SKILL.md`. That is the layout defined by
the [Agent Skills specification](https://agentskills.io/specification).

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

Either form is safe against the files an operating system leaves behind.
`SkillsFromFS` refuses a skill holding `.DS_Store`, `Thumbs.db`, `desktop.ini`
or `.localized`, and names the file. Those were committed, and they would ship
to everyone.

An installed skill is different. It sits in a directory a user may open in a
file browser, so the same files are ignored there. A `.DS_Store` appearing
beside an installed skill does not make it read as `modified`.

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

Five of the six share `.agents/skills` at project scope. Selecting several of
them resolves to one directory. Each skill is written there once.

`--agent` also takes two words.

| Value | Meaning |
| --- | --- |
| `detected` | The agents whose directory is already there. The default |
| `all` | Every agent, present or not |

`detected` falls back to `all` when it finds nothing, so a fresh repository
still gets its skills. In a repository that already holds `.claude`, only
Claude Code is written to. In a home directory it is the agents in use, rather
than six directories of which most are litter.

> [!NOTE]
> `gh skill install` defaults to `github-copilot`, and prompts for the agent
> when it can. A tool that embeds its skills is rarely able to prompt, and that
> default writes only `.agents/skills`, which Claude Code does not read.
> `WithDefaultAgents` changes this if you want the `gh` behaviour.

Claude Code moves its whole configuration with `CLAUDE_CONFIG_DIR`. User scope
follows that variable when it is set.

## The command

```
$ mytool skill
Manage the agent skills embedded in mytool.

Usage:
  mytool skill install   [flags] [skill...]
  mytool skill uninstall [flags] [skill...]
  mytool skill list      [flags] [skill...]

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
  -scope string
    	Installation scope: {project|user} (default "project")

```

`mytool skill install -h` answers the same way, for that subcommand alone.

> [!NOTE]
> The frame is this library's. The flag block is the `flag` package's own, so
> it prints one dash and sits next to your tool's flags without looking
> foreign. Both `-agent` and `--agent` are accepted, as always with that
> package. `-f` and `-force` are two flags on one variable, which is why they
> print on two lines.
>
> The cobra and urfave/cli adapters print `--agent`, because that is what
> those frameworks print.

> [!WARNING]
> Your tool's own help says nothing about the skill command. `Intercept` runs
> before your flags are even defined, and it cannot reach `flag.Usage` or an
> analyzer's `Doc`. Nobody finds the command unless you name it.

`UsageHint` is that line. It tracks the command name and the skill count, so
it cannot drift from what the command actually does.

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
| `outdated` | Your binary carries a newer copy | Overwrites it |
| `modified` | The user edited it after installing | Refuses without `--force` |
| `foreign` | Another tool owns that name | Refuses without `--force` |

> [!WARNING]
> `WithMetadata(false)` turns the four keys off. Install can then no longer
> tell an outdated copy from an edited one. Every existing directory reads as
> `foreign`.

## Frameworks

The core module has no dependencies beyond the standard library. Each adapter
is a module of its own, so embedding skills never pulls cobra into your linter.

| Framework | Module | How |
| --- | --- | --- |
| stdlib `flag` | core | `skills.Intercept()` |
| `singlechecker`, `multichecker`, `unitchecker` | core | `skills.Intercept()` |
| cobra | `github.com/mpyw/go-skill-embed/skillcobra` | `root.AddCommand(skillcobra.Command(skills))` |
| urfave/cli v3 | `github.com/mpyw/go-skill-embed/skillurfavev3` | `skillurfavev3.Command(skills)` |
| urfave/cli v2 | `github.com/mpyw/go-skill-embed/skillurfavev2` | `skillurfavev2.Command(skills)` |

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

### cobra

```go
root.AddCommand(skillcobra.Command(skills))
```

### urfave/cli

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
| `WithAgents` | All six | Restricts what `--agent` accepts |
| `WithDefaultAgents` | `github-copilot` | Used when `--agent` is absent |
| `WithDefaultScope` | `project` | Used when `--scope` is absent |
| `WithProjectRoot` | The working directory | What project scope resolves against |
| `WithMetadata` | On | Writes the four `x-embedded-*` keys |
| `WithExecutable` | Shebang test | Decides which files become executable |
| `WithOutput` | `os.Stdout` | Where `Run` reports |

> [!CAUTION]
> `embed.FS` does not carry file modes. Every embedded file arrives read-only.
> A script installed without repair cannot be run by the agent.
> The default marks any file starting with `#!` as executable.
> Pass `WithExecutable` when your scripts have no shebang.

## Using it as a library

`Run` never calls `os.Exit`, so a driver keeps control.

```go
err := skills.Install(skillembed.InstallOptions{
	Agents: []string{"claude-code"},
	Scope:  "user",
})
```

`Status` reports without changing anything. `Install` and `Uninstall` return
one `InstallResult` per skill per destination.

## Development

Tools are pinned in `mise.toml`.

```bash
mise install
./test_all.sh
```

Declaration scopes are enforced by [declscope](https://github.com/mpyw/declscope),
at `qualify: ondemand` with `exported: true`. The settings are in
`.declscope.yaml`.

## License

MIT
