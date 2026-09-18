// Package skillurfavev2 exposes an embedded skill set as an urfave/cli v2 command.
//
//	app := &cli.App{
//		Name:     "mytool",
//		Commands: []*cli.Command{skillurfavev2.Command(skills)},
//	}
//
// The flags are the ones `gh skill install` defines, so a user who knows that
// command already knows this one. Use the skillurfavev3 module for v3.
package skillurfavev2

import (
	"io"
	"os"

	"github.com/urfave/cli/v2"

	skillembed "github.com/mpyw/go-skill-embed"
)

// Command returns the skill command, with install, uninstall and list beneath
// it. Its name is the installer's command name, "skill" unless it was changed.
func Command(in *skillembed.Installer) *cli.Command {
	return &cli.Command{
		Name:  in.CommandName(),
		Usage: "Manage the skills embedded in " + in.ToolName(),
		Subcommands: []*cli.Command{
			action(in, "install", "Install the embedded skills", in.Install),
			action(in, "uninstall", "Remove the embedded skills", in.Uninstall),
			list(in),
		},
	}
}

// action builds install or uninstall, which differ only in what they call.
func action(in *skillembed.Installer, name, usage string, run func(skillembed.InstallOptions) ([]skillembed.InstallResult, error)) *cli.Command {
	return &cli.Command{
		Name:      name,
		Usage:     usage,
		ArgsUsage: "[skill...]",
		Flags:     flags(in),
		Action: func(c *cli.Context) error {
			o := options(c)
			// Report first: the results describe everything that happened
			// before the error.
			results, runErr := run(o)
			if _, err := io.WriteString(writer(c), skillembed.RenderCLIResults(results, o.DryRun)); err != nil {
				return err
			}
			return runErr
		},
	}
}

func list(in *skillembed.Installer) *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "Show the embedded skills and where they stand",
		ArgsUsage: "[skill...]",
		Flags:     flags(in),
		Action: func(c *cli.Context) error {
			statuses, err := in.Status(options(c))
			if err != nil {
				return err
			}
			_, err = io.WriteString(writer(c), skillembed.RenderCLIStatus(statuses))
			return err
		},
	}
}

// flags are the ones every subcommand shares.
func flags(in *skillembed.Installer) []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{Name: "agent", Usage: "Target agent: " + in.AgentChoices() + ", or all, or detected"},
		&cli.StringFlag{Name: "dir", Usage: "Install to a custom directory (overrides --agent and --scope)"},
		&cli.StringFlag{Name: "scope", Value: string(in.DefaultScope()), Usage: "Installation scope: {project|user}"},
		&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "Overwrite existing skills"},
		&cli.BoolFlag{Name: "dry-run", Usage: "Report what would happen without writing"},
	}
}

func options(c *cli.Context) skillembed.InstallOptions {
	return skillembed.InstallOptions{
		Agents: c.StringSlice("agent"),
		Scope:  skillembed.Scope(c.String("scope")),
		Dir:    c.String("dir"),
		Force:  c.Bool("force"),
		DryRun: c.Bool("dry-run"),
		Names:  c.Args().Slice(),
	}
}

func writer(c *cli.Context) io.Writer {
	if c.App != nil && c.App.Writer != nil {
		return c.App.Writer
	}
	return os.Stdout
}
