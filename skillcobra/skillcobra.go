// Package skillcobra exposes an embedded skill set as a cobra command.
//
//	root.AddCommand(skillcobra.Command(skills))
//
// The flags are the ones `gh skill install` defines, so a user who knows that
// command already knows this one.
package skillcobra

import (
	"io"

	"github.com/spf13/cobra"

	skillembed "github.com/mpyw/go-skill-embed"
)

// Command returns the skill command, with install, uninstall and list beneath
// it. Its name is the installer's command name, "skill" unless it was changed.
func Command(in *skillembed.Installer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   in.CommandName(),
		Short: "Manage the skills embedded in " + in.ToolName(),
		Long: "Install the agent skills that are compiled into " + in.ToolName() +
			".\n\nSkills are written to a directory the agent reads, at project or user scope.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		action(in, "install", "Install the embedded skills", in.Install),
		action(in, "uninstall", "Remove the embedded skills", in.Uninstall),
		list(in),
	)
	return cmd
}

// action builds install or uninstall, which differ only in what they call and
// what they print.
func action(in *skillembed.Installer, name, short string, run func(skillembed.InstallOptions) ([]skillembed.InstallResult, error)) *cobra.Command {
	var o skillembed.InstallOptions
	cmd := &cobra.Command{
		Use:   name + " [skill...]",
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Names = args
			// Report first: the results describe everything that happened
			// before the error.
			results, runErr := run(o)
			if _, err := io.WriteString(cmd.OutOrStdout(), skillembed.RenderCLIResults(results, o.DryRun)); err != nil {
				return err
			}
			return runErr
		},
	}
	bind(cmd, in, &o)
	return cmd
}

func list(in *skillembed.Installer) *cobra.Command {
	var o skillembed.InstallOptions
	cmd := &cobra.Command{
		Use:   "list [skill...]",
		Short: "Show the embedded skills and where they stand",
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Names = args
			statuses, err := in.Status(o)
			if err != nil {
				return err
			}
			_, err = io.WriteString(cmd.OutOrStdout(), skillembed.RenderCLIStatus(statuses))
			return err
		},
	}
	bind(cmd, in, &o)
	return cmd
}

// bind registers the flags shared by every subcommand.
func bind(cmd *cobra.Command, in *skillembed.Installer, o *skillembed.InstallOptions) {
	f := cmd.Flags()
	f.StringSliceVar(&o.Agents, "agent", nil, "Target agent: "+in.AgentChoices()+", or all, or detected")
	f.StringVar(&o.Dir, "dir", "", "Install to a custom directory (overrides --agent and --scope)")
	f.StringVar(&o.Scope, "scope", string(in.DefaultScope()), "Installation scope: {project|user}")
	f.BoolVarP(&o.Force, "force", "f", false, "Overwrite existing skills")
	f.BoolVar(&o.DryRun, "dry-run", false, "Report what would happen without writing")
}
