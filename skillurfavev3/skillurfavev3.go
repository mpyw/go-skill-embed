// Package skillurfavev3 exposes an embedded skill set as an urfave/cli v3 command.
//
//	app := &cli.Command{
//		Name:     "mytool",
//		Commands: []*cli.Command{skillurfavev3.Command(skills)},
//	}
//
// The flags are the ones `gh skill install` defines, so a user who knows that
// command already knows this one.
package skillurfavev3

import (
	"context"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	skillembed "github.com/mpyw/go-skill-embed"
)

// Command returns the skill command, with install, uninstall and list beneath
// it. Its name is the installer's command name, "skill" unless it was changed.
func Command(in *skillembed.Installer) *cli.Command {
	return &cli.Command{
		Name:  in.CommandName(),
		Usage: "Manage the skills embedded in " + in.ToolName(),
		Commands: []*cli.Command{
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
		Action: func(_ context.Context, cmd *cli.Command) error {
			o := options(cmd)
			results, err := run(o)
			if err != nil {
				return err
			}
			_, err = io.WriteString(writer(cmd), skillembed.RenderCLIResults(results, o.DryRun))
			return err
		},
	}
}

func list(in *skillembed.Installer) *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "Show the embedded skills and where they stand",
		ArgsUsage: "[skill...]",
		Flags:     flags(in),
		Action: func(_ context.Context, cmd *cli.Command) error {
			statuses, err := in.Status(options(cmd))
			if err != nil {
				return err
			}
			_, err = io.WriteString(writer(cmd), skillembed.RenderCLIStatus(statuses))
			return err
		},
	}
}

// flags are the ones every subcommand shares.
func flags(in *skillembed.Installer) []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{Name: "agent", Usage: "Target agent: " + in.AgentChoices() + ", or all"},
		&cli.StringFlag{Name: "dir", Usage: "Install to a custom directory (overrides --agent and --scope)"},
		&cli.StringFlag{Name: "scope", Value: string(in.DefaultScope()), Usage: "Installation scope: {project|user}"},
		&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "Overwrite existing skills"},
		&cli.BoolFlag{Name: "dry-run", Usage: "Report what would happen without writing"},
	}
}

func options(cmd *cli.Command) skillembed.InstallOptions {
	return skillembed.InstallOptions{
		Agents: cmd.StringSlice("agent"),
		Scope:  cmd.String("scope"),
		Dir:    cmd.String("dir"),
		Force:  cmd.Bool("force"),
		DryRun: cmd.Bool("dry-run"),
		Names:  cmd.Args().Slice(),
	}
}

func writer(cmd *cli.Command) io.Writer {
	if w := cmd.Root().Writer; w != nil {
		return w
	}
	return os.Stdout
}
