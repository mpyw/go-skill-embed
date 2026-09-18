package skillembed

import (
	"bytes"
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"
	"unicode/utf8"
)

// usageWidth is how much of a description the listings show.
const usageWidth = 100

// Usage is the help text for the skill command. A tool that writes its own
// help can print it, so that the two agree.
func (in *Installer) Usage() string {
	var o InstallOptions
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
	if in.Set().Len() != 1 {
		skills = "skills"
	}
	return fmt.Sprintf("Run %q to install the %d agent %s embedded in %s.",
		in.ToolName()+" "+in.CommandName(), in.Set().Len(), skills, in.ToolName())
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
// it cannot drift from it, and it reads the way a flag package tool reads. The
// long forms work too, since the flag package accepts either, but a tool whose
// own flags print as -v should not print --agent next to them.
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
	for _, sk := range in.Set().Skills() {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\n", sk.Name, usageFirstLine(sk.Description))
	}
	_ = tw.Flush()
	return usageTrimLines(b.String())
}

// usageTrimLines removes the padding a tabwriter leaves at the end of a line
// when the last column is empty. Nothing should print trailing whitespace, and
// an Output comment in an example cannot carry it either.
//
//declscope:package // every rendered block goes through it, here and in cli.go
func usageTrimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// usageFirstLine shortens a description to one line that fits a table.
//
//declscope:package // the skill listing in cli.go prints descriptions too
func usageFirstLine(s string) string {
	if i := strings.IndexAny(s, "\n\r"); i >= 0 {
		s = s[:i]
	}
	// A tab would be read as a column separator by the tabwriter this feeds.
	s = strings.ReplaceAll(s, "\t", " ")
	// Counted in runes, and cut on a rune boundary. Bytes would cut a CJK
	// description at a third of the length, and in the middle of a character.
	if utf8.RuneCountInString(s) > usageWidth {
		return string([]rune(s)[:usageWidth-3]) + "..."
	}
	return s
}
