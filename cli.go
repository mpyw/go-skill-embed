package skillembed

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// ErrHelp is returned by Run when help was requested. It is not a failure.
//
//declscope:ignore qualify // written by hand in the user's main, where CliIntercept would read worse
var ErrHelp = flag.ErrHelp

// Run executes the skill command. args are the arguments after the command
// name, so `mytool skill install --scope user` passes
// []string{"install", "--scope", "user"}.
//
// It never calls os.Exit, so a driver can keep control. Run is what the cobra
// and urfave/cli adapters call underneath, and what Intercept wraps.
//
//declscope:ignore qualify // written by hand in the user's main, where CliIntercept would read worse
func (in *Installer) Run(args []string) error {
	if len(args) == 0 {
		_, _ = io.WriteString(in.Output(), in.renderCLIUsage())
		return ErrHelp
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "install":
		return in.cliInstall(rest)
	case "uninstall", "remove":
		return in.cliUninstall(rest)
	case "list", "ls":
		return in.cliList(rest)
	case "help", "-h", "--help":
		_, _ = io.WriteString(in.Output(), in.renderCLIUsage())
		return nil
	}
	_, _ = io.WriteString(os.Stderr, in.renderCLIUsage())
	return fmt.Errorf("unknown %s subcommand %q", in.CommandName(), sub)
}

// Intercept runs the skill command when it is the first argument, and exits.
//
// Call it as the first statement of main, before any driver that parses the
// command line itself. singlechecker, unitchecker and multichecker all treat
// every non-flag argument as a package pattern, so there is no later point at
// which a subcommand can still be recognised:
//
//	func main() {
//		skills.Intercept()
//		singlechecker.Main(mylint.Analyzer)
//	}
//
// The guard is the literal first argument, so `go vet -vettool=` invocations,
// which pass -flags or a config file path, are never affected.
//
//declscope:ignore qualify // written by hand in the user's main, where CliIntercept would read worse
func (in *Installer) Intercept() {
	handled, err := in.cliIntercept(os.Args)
	if !handled {
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", in.ToolName(), err)
		os.Exit(1)
	}
	os.Exit(0)
}

// cliIntercept holds everything about Intercept that can be decided without
// exiting the process, so that a test can reach it.
func (in *Installer) cliIntercept(argv []string) (handled bool, err error) {
	if len(argv) < 2 || argv[1] != in.CommandName() {
		return false, nil
	}
	if err := in.Run(argv[2:]); err != nil && !errors.Is(err, ErrHelp) {
		return true, err
	}
	return true, nil
}

// cliRepeatable collects a flag that may be given more than once, and that also
// accepts a comma separated list.
type cliRepeatable []string

func (r *cliRepeatable) String() string { return strings.Join(*r, ",") }

func (r *cliRepeatable) Set(v string) error {
	*r = append(*r, v)
	return nil
}

// bindCLIFlags registers the flags `gh skill install` defines, so the two read
// the same way.
func (in *Installer) bindCLIFlags(fs *flag.FlagSet, o *InstallOptions, agents *cliRepeatable) {
	fs.Var(agents, "agent", "Target agent: "+in.AgentChoices()+" (repeatable, or all)")
	fs.StringVar(&o.Dir, "dir", "", "Install to a custom directory (overrides --agent and --scope)")
	fs.StringVar(&o.Scope, "scope", "", fmt.Sprintf("Installation scope: {project|user} (default %q)", in.DefaultScope()))
	fs.BoolVar(&o.Force, "force", false, "Overwrite existing skills")
	fs.BoolVar(&o.Force, "f", false, "Overwrite existing skills (shorthand)")
	fs.BoolVar(&o.DryRun, "dry-run", false, "Report what would happen without writing")
}

func (in *Installer) newCLIFlagSet(sub string) *flag.FlagSet {
	fs := flag.NewFlagSet(in.CommandName()+" "+sub, flag.ContinueOnError)
	fs.SetOutput(in.Output())
	return fs
}

func (in *Installer) parseCLIOptions(sub string, args []string) (InstallOptions, error) {
	var o InstallOptions
	var agents cliRepeatable
	fs := in.newCLIFlagSet(sub)
	in.bindCLIFlags(fs, &o, &agents)
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	o.Agents = agents
	o.Names = fs.Args()
	return o, nil
}

func (in *Installer) cliInstall(args []string) error {
	o, err := in.parseCLIOptions("install", args)
	if err != nil {
		return err
	}
	results, err := in.Install(o)
	if err != nil {
		return err
	}
	_, err = io.WriteString(in.Output(), RenderCLIResults(results, o.DryRun))
	return err
}

func (in *Installer) cliUninstall(args []string) error {
	o, err := in.parseCLIOptions("uninstall", args)
	if err != nil {
		return err
	}
	results, err := in.Uninstall(o)
	if err != nil {
		return err
	}
	_, err = io.WriteString(in.Output(), RenderCLIResults(results, o.DryRun))
	return err
}

func (in *Installer) cliList(args []string) error {
	o, err := in.parseCLIOptions("list", args)
	if err != nil {
		return err
	}
	statuses, err := in.Status(o)
	if err != nil {
		return err
	}
	_, err = io.WriteString(in.Output(), in.renderCLIList(statuses))
	return err
}

// renderCLIList describes the embedded skills and where each one stands.
func (in *Installer) renderCLIList(statuses []InstallStatus) string {
	var b bytes.Buffer
	for _, sk := range in.Set().Skills() {
		fmt.Fprintf(&b, "%s\n", sk.Name)
		if sk.Description != "" {
			fmt.Fprintf(&b, "  %s\n", cliFirstLine(sk.Description))
		}
	}
	b.WriteByte('\n')
	b.WriteString(RenderCLIStatus(statuses))
	return b.String()
}

// RenderCLIStatus is the table list prints. The adapters use it so that every
// front end reports the same way.
func RenderCLIStatus(statuses []InstallStatus) string {
	var b bytes.Buffer
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SKILL\tSTATE\tPATH")
	for _, st := range statuses {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", st.Skill.Name, st.State, st.Path)
	}
	_ = tw.Flush()
	return cliTrimLines(b.String())
}

// RenderCLIResults lists what install or uninstall did at each destination. The
// adapters use it so that every front end reports the same way.
func RenderCLIResults(results []InstallResult, dryRun bool) string {
	prefix := ""
	if dryRun {
		prefix = "would be "
	}
	var b bytes.Buffer
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, r := range results {
		note := ""
		if r.Reason != "" {
			note = " (" + r.Reason + ")"
		}
		_, _ = fmt.Fprintf(tw, "%s%s\t%s\t%s\n", prefix, r.Action, r.Skill.Name, r.Path+note)
	}
	_ = tw.Flush()
	return cliTrimLines(b.String())
}

// renderCLIUsage is the help text. It mirrors the flags `gh skill install`
// defines, so the two read the same way.
func (in *Installer) renderCLIUsage() string {
	var b bytes.Buffer
	name := in.CommandName()
	fmt.Fprintf(&b, `Manage the skills embedded in %s.

Usage:
  %s %s install   [flags] [skill...]
  %s %s uninstall [flags] [skill...]
  %s %s list      [flags] [skill...]

Flags:
      --agent string   Target agent: %s (repeatable, or all) (default %q)
      --dir string     Install to a custom directory (overrides --agent and --scope)
  -f, --force          Overwrite existing skills
      --scope string   Installation scope: {project|user} (default %q)
      --dry-run        Report what would happen without writing

Embedded skills:
`,
		in.ToolName(),
		in.ToolName(), name,
		in.ToolName(), name,
		in.ToolName(), name,
		in.AgentChoices(), strings.Join(in.DefaultAgentNames(), ","),
		in.DefaultScope(),
	)
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, sk := range in.Set().Skills() {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\n", sk.Name, cliFirstLine(sk.Description))
	}
	_ = tw.Flush()
	return cliTrimLines(b.String())
}

// cliTrimLines removes the padding a tabwriter leaves at the end of a line
// when the last column is empty. Nothing should print trailing whitespace, and
// an Output comment in an example cannot carry it either.
func cliTrimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func cliFirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 100 {
		return s[:97] + "..."
	}
	return s
}
