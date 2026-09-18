package skillembed

import (
	"bytes"
	"context"
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
func (in *Installer) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		_, _ = io.WriteString(in.cliOut(), in.Usage())
		return ErrHelp
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "install":
		return in.cliInstall(ctx, rest)
	case "uninstall", "remove":
		return in.cliUninstall(ctx, rest)
	case "list", "ls":
		return in.cliList(ctx, rest)
	case "help", "-h", "--help":
		_, _ = io.WriteString(in.cliOut(), in.Usage())
		return ErrHelp
	}
	_, _ = io.WriteString(in.cliErrOut(), in.Usage())
	return fmt.Errorf("unknown %s subcommand %q", in.CommandName(), sub)
}

// Intercept runs the skill command when it is the first argument, and exits.
//
// Call it as the first statement of main, before flag.Parse or any driver that
// reads the command line itself:
//
//	func main() {
//		skills.Intercept()
//		flag.Parse()
//		// the rest of your tool
//	}
//
// The guard is the first argument and nothing else. `mytool skill install`
// reaches it. `mytool -v skill install` does not. A `go vet -vettool=`
// invocation passes -flags or a config file path, so it is never affected.
//
// singlechecker, unitchecker and multichecker read every non-flag argument as
// a package pattern, and they parse the command line themselves. No later
// point exists at which a subcommand could still be recognised, so Intercept
// has to run before them.
//
//declscope:ignore qualify // written by hand in the user's main, where CliIntercept would read worse
func (in *Installer) Intercept() {
	handled, err := in.cliIntercept(context.Background(), os.Args)
	if !handled {
		return
	}
	if err != nil {
		_, _ = fmt.Fprintf(in.cliErrOut(), "%s: %v\n", in.ToolName(), err)
		os.Exit(1)
	}
	os.Exit(0)
}

// cliIntercept holds everything about Intercept that can be decided without
// exiting the process, so that a test can reach it.
func (in *Installer) cliIntercept(ctx context.Context, argv []string) (handled bool, err error) {
	if len(argv) < 2 || argv[1] != in.CommandName() {
		return false, nil
	}
	if err := in.Run(ctx, argv[2:]); err != nil && !errors.Is(err, ErrHelp) {
		return true, err
	}
	return true, nil
}

// cliRepeatable is a flag that may be given more than once, and that also
// accepts a comma separated list. It appends straight into the options, so no
// caller has to hold an intermediate slice.
type cliRepeatable struct{ dest *[]string }

// String is called on a zero value to decide whether a default is worth
// printing, so it has to survive a nil destination.
func (r cliRepeatable) String() string {
	if r.dest == nil {
		return ""
	}
	return strings.Join(*r.dest, ",")
}

func (r cliRepeatable) Set(v string) error {
	*r.dest = append(*r.dest, v)
	return nil
}

// cliOut is where Run reports. It defaults to os.Stdout.
func (in *Installer) cliOut() io.Writer {
	if in.out == nil {
		return os.Stdout
	}
	return in.out
}

// cliErrOut is where Run reports a mistake. It defaults to os.Stderr.
func (in *Installer) cliErrOut() io.Writer {
	if in.errOut == nil {
		return os.Stderr
	}
	return in.errOut
}

// cliScope binds the typed Scope to a string flag.
//
// It stores the value without checking it. The flag package reformats an error
// from Set with %v, which drops the wrapped ErrUnknownScope, so the check is
// left to Targets.
type cliScope struct{ dest *Scope }

// String is called on a zero value to decide whether a default is worth
// printing, so it has to survive a nil destination.
func (s cliScope) String() string {
	if s.dest == nil {
		return ""
	}
	return string(*s.dest)
}

func (s cliScope) Set(v string) error {
	*s.dest = Scope(v)
	return nil
}

// bindCLIFlags registers the flags `gh skill install` defines, so the two read
// the same way.
func (in *Installer) bindCLIFlags(fs *flag.FlagSet, o *InstallOptions) {
	fs.Var(cliRepeatable{&o.Agents}, "agent", fmt.Sprintf("Target agent: %s, or all, or detected (repeatable) (default %q)",
		in.AgentChoices(), strings.Join(in.defaultAgent, ",")))
	fs.StringVar(&o.Dir, "dir", "", "Install to a custom directory (overrides -agent and -scope)")
	fs.Var(cliScope{&o.Scope}, "scope", "Installation scope: {project|user}")
	fs.BoolVar(&o.Force, "force", false, "Overwrite existing skills")
	fs.BoolVar(&o.Force, "f", false, "Overwrite existing skills (shorthand)")
	fs.BoolVar(&o.DryRun, "dry-run", false, "Report what would happen without writing")
}

// newCLIFlagSet builds the FlagSet for one subcommand, already bound and
// already knowing how to print itself.
//
//declscope:package // usage.go renders the flag block from the real FlagSet
func (in *Installer) newCLIFlagSet(sub string, o *InstallOptions) *flag.FlagSet {
	fs := flag.NewFlagSet(in.CommandName()+" "+sub, flag.ContinueOnError)
	// The flag package would print the complaint and then call Usage, and the
	// caller prints the complaint again. Both are silenced here and written
	// once, by parseCLIOptions, to the stream each belongs on.
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	in.bindCLIFlags(fs, o)
	return fs
}

func (in *Installer) parseCLIOptions(sub string, args []string) (InstallOptions, error) {
	o := InstallOptions{Scope: in.DefaultScope()}
	fs := in.newCLIFlagSet(sub, &o)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			// Help was asked for, so it is what was wanted.
			_, _ = io.WriteString(in.cliOut(), in.usageFor(sub, fs))
			return o, ErrHelp
		}
		_, _ = io.WriteString(in.cliErrOut(), in.usageFor(sub, fs))
		return o, err
	}
	o.Names = fs.Args()
	return o, nil
}

func (in *Installer) cliInstall(ctx context.Context, args []string) error {
	o, err := in.parseCLIOptions("install", args)
	if err != nil {
		return err
	}
	// Report first. Install describes everything it did before the error, and a
	// run that wrote three skills and refused a fourth has to say so.
	results, runErr := in.Install(ctx, o)
	if _, err := io.WriteString(in.cliOut(), RenderCLIResults(results, o.DryRun)); err != nil {
		return err
	}
	return runErr
}

func (in *Installer) cliUninstall(ctx context.Context, args []string) error {
	o, err := in.parseCLIOptions("uninstall", args)
	if err != nil {
		return err
	}
	results, runErr := in.Uninstall(ctx, o)
	if _, err := io.WriteString(in.cliOut(), RenderCLIResults(results, o.DryRun)); err != nil {
		return err
	}
	return runErr
}

func (in *Installer) cliList(ctx context.Context, args []string) error {
	o, err := in.parseCLIOptions("list", args)
	if err != nil {
		return err
	}
	statuses, err := in.Status(ctx, o)
	if err != nil {
		return err
	}
	_, err = io.WriteString(in.cliOut(), RenderCLIStatus(statuses))
	return err
}

// RenderCLIStatus is everything `list` prints: each skill once with its
// description, then a row per destination.
//
// It is the whole of the subcommand's output, not a part of it, so that every
// front end that calls it says the same thing. The skills come from the
// statuses rather than from the set, so a run narrowed by name describes only
// what it was asked about.
func RenderCLIStatus(statuses []InstallStatus) string {
	var b bytes.Buffer

	seen := map[string]bool{}
	for _, st := range statuses {
		if seen[st.Skill.Name] {
			continue
		}
		seen[st.Skill.Name] = true
		fmt.Fprintf(&b, "%s\n", st.Skill.Name)
		if st.Skill.Description != "" {
			fmt.Fprintf(&b, "  %s\n", usageFirstLine(st.Skill.Description))
		}
	}
	b.WriteByte('\n')

	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SKILL\tSTATE\tPATH")
	for _, st := range statuses {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", st.Skill.Name, st.State, st.Path)
	}
	_ = tw.Flush()
	return usageTrimLines(b.String())
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
	return usageTrimLines(b.String())
}
