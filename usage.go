package skillembed

import (
	"bytes"
	"flag"
	"fmt"
	"text/tabwriter"

	"github.com/mpyw/go-skill-embed/internal/textfmt"
)

// Usage is the help text for the skill command. A tool that writes its own
// help can print it, so that the two agree.
func (in *Installer) Usage() string {
	o := InstallOptions{Scope: in.DefaultScope()}
	return in.usageFor("", in.newCLIFlagSet("", &o))
}

// UsageHint is one line naming the skill command, for a tool whose own help
// would otherwise never mention it. flag.Usage and an analyzer's Doc are the
// two places it belongs.
//
//	flag.Usage = func() {
//		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
//		flag.PrintDefaults()
//		fmt.Fprintf(os.Stderr, "\n%s\n", skills.UsageHint())
//	}
func (in *Installer) UsageHint() string {
	skills := "skill"
	if in.set.Len() != 1 {
		skills = "skills"
	}
	return fmt.Sprintf("Run %q to install the %d agent %s embedded in %s.",
		in.ToolName()+" "+in.CommandName(), in.set.Len(), skills, in.ToolName())
}

// usageHeadings are the first line and the usage line of each subcommand's
// help. An empty key is the overview.
func (in *Installer) usageHeadings(sub string) (summary, lines string) {
	tool, cmd := in.ToolName(), in.CommandName()
	switch sub {
	case "install":
		return "Install the agent skills embedded in " + tool + ".",
			fmt.Sprintf("  %s %s install [flags] [skill...]\n", tool, cmd)
	case "uninstall":
		return "Remove the agent skills embedded in " + tool + ".",
			fmt.Sprintf("  %s %s uninstall [flags] [skill...]\n", tool, cmd)
	case "list":
		return "Show the agent skills embedded in " + tool + ", and where each one stands.",
			fmt.Sprintf("  %s %s list [flags] [skill...]\n", tool, cmd)
	}
	return "Manage the agent skills embedded in " + tool + ".",
		fmt.Sprintf("  %s %s install   [flags] [skill...]\n", tool, cmd) +
			fmt.Sprintf("  %s %s uninstall [flags] [skill...]\n", tool, cmd) +
			fmt.Sprintf("  %s %s list      [flags] [skill...]\n", tool, cmd)
}

// usageFor renders the help for one subcommand, or for the command itself when
// sub is empty.
//
// The flag block comes from the FlagSet that actually parses the arguments, so
// it cannot drift from it. It also reads the way a flag package tool reads.
// The long forms work as well, since the flag package accepts either. A tool
// whose own flags print as -v should not print --agent next to them.
//
//declscope:package // cli.go installs it as each subcommand's flag.Usage
func (in *Installer) usageFor(sub string, fs *flag.FlagSet) string {
	summary, lines := in.usageHeadings(sub)

	var b bytes.Buffer
	fmt.Fprintf(&b, "%s\n\nUsage:\n%s\nFlags:\n", summary, lines)

	restore := fs.Output()
	fs.SetOutput(&b)
	fs.PrintDefaults()
	fs.SetOutput(restore)

	b.WriteString("\nEmbedded skills:\n")

	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, sk := range in.set.Skills() {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\n", sk.Name, textfmt.FirstLine(sk.Description))
	}
	_ = tw.Flush()
	return textfmt.TrimLines(b.String())
}
