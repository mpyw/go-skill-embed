package skillembed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mpyw/go-skill-embed/internal/manifest"
	"github.com/mpyw/go-skill-embed/internal/projectroot"
	"github.com/mpyw/go-skill-embed/internal/skillfs"
)

// Installer installs embedded skills into agent directories.
type Installer struct {
	toolName     string
	version      string
	commandName  string
	defaultScope Scope
	metadata     bool
	executable   func(name string, data []byte) bool
	now          func() time.Time

	// What another file reads. cli.go and usage.go build help and choose a
	// writer, agent.go renders the agent choices, and project.go searches for
	// a root when projectRoot is empty. The fields above are install.go's own,
	// and declscope reports it if one of them is read elsewhere.
	//declscope:package
	set *SkillSet
	//declscope:package
	agents []Agent
	//declscope:package
	defaultAgent []AgentSelector
	//declscope:package
	projectRoot string
	//declscope:package
	out io.Writer
	//declscope:package
	errOut io.Writer
}

// InstallerOption configures an Installer.
type InstallerOption func(*Installer)

// WithToolName sets the name recorded in installed skills and shown in help.
// It defaults to the running binary's name.
//
//declscope:ignore qualify // With* is Go's option idiom, and InstallerWithToolName reads worse at every call site
func WithToolName(name string) InstallerOption { return func(i *Installer) { i.toolName = name } }

// WithVersion sets the version recorded in installed skills.
//
//declscope:ignore qualify // With* is Go's option idiom, and InstallerWithToolName reads worse at every call site
func WithVersion(v string) InstallerOption { return func(i *Installer) { i.version = v } }

// WithCommandName sets the subcommand name used by Run and Intercept. It
// defaults to "skill".
//
//declscope:ignore qualify // With* is Go's option idiom, and InstallerWithToolName reads worse at every call site
func WithCommandName(name string) InstallerOption { return func(i *Installer) { i.commandName = name } }

// WithAgents restricts the agents the tool offers. It defaults to every
// built-in agent.
//
//declscope:ignore qualify // With* is Go's option idiom, and InstallerWithToolName reads worse at every call site
func WithAgents(agents ...Agent) InstallerOption {
	return func(i *Installer) { i.agents = append([]Agent(nil), agents...) }
}

// WithDefaultAgents sets the agents used when --agent is not given. It
// defaults to "detected".
//
// "detected" keeps the agents whose directory is already there, and falls back
// to "all" when it finds none. In a fresh repository that is two directories
// and reaches everything. In a home directory it is the agents in use, rather
// than six directories of which most are litter. Pass "github-copilot" for the
// default `gh skill install` uses.
//
//declscope:ignore qualify // With* is Go's option idiom, and InstallerWithToolName reads worse at every call site
func WithDefaultAgents(selectors ...AgentSelector) InstallerOption {
	return func(i *Installer) { i.defaultAgent = append([]AgentSelector(nil), selectors...) }
}

// WithDefaultScope sets the scope used when --scope is not given. It defaults
// to project, matching `gh skill install`.
//
//declscope:ignore qualify // With* is Go's option idiom, and InstallerWithToolName reads worse at every call site
func WithDefaultScope(s Scope) InstallerOption { return func(i *Installer) { i.defaultScope = s } }

// WithProjectRoot sets the directory project scope resolves against.
//
// Without it the root is searched for: the walk starts at the working
// directory and stops at the repository root, and the first directory already
// holding an agent directory wins. Outside a repository the working directory
// is the only candidate. The home directory is refused, since the user scope
// directories live there.
//
//declscope:ignore qualify // With* is Go's option idiom, and InstallerWithToolName reads worse at every call site
func WithProjectRoot(dir string) InstallerOption { return func(i *Installer) { i.projectRoot = dir } }

// WithMetadata controls whether installed skills carry x-embedded-* frontmatter.
// Without it, install cannot tell an outdated copy from an edited one and every
// existing directory reads as foreign. It is on by default.
//
//declscope:ignore qualify // With* is Go's option idiom, and InstallerWithToolName reads worse at every call site
func WithMetadata(on bool) InstallerOption { return func(i *Installer) { i.metadata = on } }

// WithExecutable decides which files are written with the executable bit.
// embed.FS does not carry file modes, so the default marks any file starting
// with a #! shebang.
//
//declscope:ignore qualify // With* is Go's option idiom, and InstallerWithToolName reads worse at every call site
func WithExecutable(fn func(name string, data []byte) bool) InstallerOption {
	return func(i *Installer) { i.executable = fn }
}

// WithOutput sets where Run writes what was asked for: the report, and the
// help text when help was requested. It defaults to os.Stdout.
//
//declscope:ignore qualify // With* is Go's option idiom, and InstallerWithToolName reads worse at every call site
func WithOutput(w io.Writer) InstallerOption { return func(i *Installer) { i.out = w } }

// WithErrorOutput sets where Run writes diagnostics: the message for a bad
// flag or an unknown subcommand, and the usage that goes with it. It defaults
// to os.Stderr.
//
// The two are separate for the sake of redirection. `mytool skill list >
// skills.txt` puts the list in the file and the complaint on the terminal.
//
//declscope:ignore qualify // With* is Go's option idiom, and InstallerWithToolName reads worse at every call site
func WithErrorOutput(w io.Writer) InstallerOption { return func(i *Installer) { i.errOut = w } }

// NewInstaller creates an Installer for a set of embedded skills.
func NewInstaller(set *SkillSet, opts ...InstallerOption) *Installer {
	in := &Installer{
		set:          set,
		commandName:  "skill",
		agents:       DefaultAgents(),
		defaultAgent: []AgentSelector{AgentSelectorDetected},
		defaultScope: ScopeProject,
		metadata:     true,
		executable:   skillfs.HasShebang,
		now:          time.Now,
	}
	for _, o := range opts {
		o(in)
	}
	if in.toolName == "" {
		in.toolName = filepath.Base(os.Args[0])
	}
	return in
}

// CommandName returns the subcommand name.
func (in *Installer) CommandName() string { return in.commandName }

// ToolName returns the name recorded in installed skills.
func (in *Installer) ToolName() string { return in.toolName }

// DefaultScope returns the scope used when none is given.
func (in *Installer) DefaultScope() Scope { return in.defaultScope }

// InstallOptions are the inputs shared by install, uninstall and list.
type InstallOptions struct {
	// Agents select the destinations. Empty means the installer default.
	Agents []AgentSelector
	// Scope is ScopeProject or ScopeUser. Empty means the installer default.
	Scope Scope
	// Dir installs into this directory, overriding Agents and Scope.
	Dir string
	// Force overwrites skills that were edited, or that something else installed.
	Force bool
	// DryRun reports what would happen without touching the file system.
	DryRun bool
	// Names selects skills by name. Empty means every embedded skill.
	Names []string
}

// InstallTarget is one destination directory and the agents that read from it.
type InstallTarget struct {
	// Dir is the absolute skills directory.
	Dir string
	// Agents read from Dir. It is empty when InstallOptions.Dir was used.
	Agents []Agent
	// Root is the project directory Dir was resolved against. It is empty for
	// user scope, and when InstallOptions.Dir named the destination outright.
	Root string
}

// Label renders the target for human readable output.
func (t InstallTarget) Label() string {
	if len(t.Agents) == 0 {
		return t.Dir
	}
	titles := make([]string, len(t.Agents))
	for i, a := range t.Agents {
		titles[i] = a.Title
	}
	return fmt.Sprintf("%s (%s)", t.Dir, strings.Join(titles, ", "))
}

// Targets resolves the destination directories for o.
//
// At project scope every agent but Claude Code shares .agents/skills, so those
// are merged into one target and a skill is never written there twice.
func (in *Installer) Targets(o InstallOptions) ([]InstallTarget, error) {
	// Validate before Dir wins, so that a bad --scope or --agent alongside it
	// is a diagnosis rather than silence.
	scope := in.defaultScope
	if o.Scope != "" {
		s, err := ParseScope(string(o.Scope))
		if err != nil {
			return nil, err
		}
		scope = s
	}

	if o.Dir != "" {
		abs, err := filepath.Abs(o.Dir)
		if err != nil {
			return nil, err
		}
		if _, err := agentsByName(in.agents, o.Agents, func(Agent) bool { return true }); err != nil {
			return nil, err
		}
		return []InstallTarget{{Dir: abs}}, nil
	}

	names := o.Agents
	if len(names) == 0 {
		names = in.defaultAgent
	}
	root, err := in.projectRootOf(scope)
	if err != nil {
		return nil, err
	}

	// An agent counts as present when the directory holding its skills
	// directory is there. The skills directory itself need not be.
	detected := func(a Agent) bool {
		dir, err := a.Dir(scope, root)
		if err != nil {
			return false
		}
		info, err := os.Stat(filepath.Dir(dir))
		return err == nil && info.IsDir()
	}

	agents, err := agentsByName(in.agents, names, detected)
	if err != nil {
		return nil, err
	}
	agents = agentsFallBackToAll(in.agents, agents, names)
	if len(agents) == 0 {
		return nil, ErrNoAgentSelected
	}

	// Only a project install has a project. User scope writes into the home
	// directory, and a named directory is the destination outright.
	stamped := ""
	if scope == ScopeProject {
		stamped = root
	}

	var targets []InstallTarget
	index := map[string]int{}
	for _, a := range agents {
		dir, err := a.Dir(scope, root)
		if err != nil {
			return nil, err
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		// Checked per agent rather than per target, so that the agent whose
		// directory escapes is the one reached before any of them is written.
		if scope == ScopeProject {
			// Only the refusal earns the advice. realPath also reports a
			// permission failure or a symlink cycle, and telling the reader to
			// pass --dir would send them past the thing that is wrong.
			if err := projectroot.Within(root, abs); errors.Is(err, projectroot.ErrOutsideRoot) {
				return nil, fmt.Errorf("skillembed: %w, so pass --dir to write"+
					" there on purpose", err)
			} else if err != nil {
				return nil, err
			}
		}
		if i, ok := index[abs]; ok {
			targets[i].Agents = append(targets[i].Agents, a)
			continue
		}
		index[abs] = len(targets)
		targets = append(targets, InstallTarget{Dir: abs, Agents: []Agent{a}, Root: stamped})
	}
	return targets, nil
}

// selected resolves o.Names against the embedded set.
func (in *Installer) selected(o InstallOptions) ([]Skill, error) {
	if len(o.Names) == 0 {
		return in.set.Skills(), nil
	}
	out := make([]Skill, 0, len(o.Names))
	for _, name := range o.Names {
		sk, ok := in.set.Lookup(name)
		if !ok {
			available := in.set.Names()
			sort.Strings(available)
			return nil, fmt.Errorf("%w %q (embedded: %s)", ErrUnknownSkill, name, strings.Join(available, ", "))
		}
		out = append(out, sk)
	}
	return out, nil
}

// InstallStatus is the state of one skill at one destination.
type InstallStatus struct {
	Skill Skill
	// Target is the destination this row is about.
	Target InstallTarget
	// Path is the skill's own directory inside InstallTarget.Dir.
	Path  string
	State State
	// InstalledBy and InstalledVersion come from the installed frontmatter.
	InstalledBy      string
	InstalledVersion string
}

// Status reports what is installed where, without changing anything.
func (in *Installer) Status(ctx context.Context, o InstallOptions) ([]InstallStatus, error) {
	targets, err := in.Targets(o)
	if err != nil {
		return nil, err
	}
	skills, err := in.selected(o)
	if err != nil {
		return nil, err
	}
	out := make([]InstallStatus, 0, len(targets)*len(skills))
	for _, t := range targets {
		for _, sk := range skills {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			st, err := in.inspect(t, sk)
			if err != nil {
				return nil, err
			}
			out = append(out, st)
		}
	}
	return out, nil
}

func (in *Installer) inspect(t InstallTarget, sk Skill) (InstallStatus, error) {
	dest := filepath.Join(t.Dir, sk.Name)
	st := InstallStatus{Skill: sk, Target: t, Path: dest, State: StateMissing}

	info, err := os.Stat(dest)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return st, nil
		}
		return st, err
	}
	if !info.IsDir() {
		st.State = StateForeign
		return st, nil
	}

	installed, err := os.ReadFile(filepath.Join(dest, SkillFile))
	if err != nil {
		// A directory with no manifest is not ours to replace silently.
		st.State = StateForeign
		return st, nil
	}
	fields := manifest.Fields(installed)
	recorded := fields[MetaKeyEmbeddedDigest]
	st.InstalledBy = fields[MetaKeyEmbeddedBy]
	st.InstalledVersion = fields[MetaKeyEmbeddedVersion]
	if recorded == "" || st.InstalledBy != in.toolName {
		st.State = StateForeign
		return st, nil
	}

	actual, err := skillfs.Digest(os.DirFS(dest))
	if err != nil {
		// Something there cannot be read or hashed, such as a symlink a user
		// dropped in. Nothing can be said about the copy, so it is not claimed
		// as this tool's, and --force stays the way out. An error here would
		// instead fail Status for every other skill at this target.
		st.State = StateForeign
		return st, nil
	}
	switch {
	case actual != recorded:
		st.State = StateModified
		return st, nil
	case recorded != sk.Digest():
		st.State = StateOutdated
		return st, nil
	}

	// The contents match. The executable bit is not in the digest, and a script
	// that lost it cannot be run by the agent. That reads as outdated rather
	// than modified, because the user did not do it and repairing it should not
	// need --force.
	ok, err := skillfs.ExecutableBitsMatch(os.DirFS(dest), in.executable)
	switch {
	case err != nil:
		st.State = StateForeign
	case !ok:
		st.State = StateOutdated
	default:
		st.State = StateUpToDate
	}
	return st, nil
}

// InstallResult is the outcome for one skill at one target.
type InstallResult struct {
	Skill Skill
	// Target is the destination this row is about.
	Target InstallTarget
	Path   string
	// Before is the state found at Path.
	Before State
	Action Action
	// Reason explains a skipped action.
	Reason string
}

// Install writes the selected skills into the resolved targets.
//
// A destination this tool did not write, or one edited after it did, is left
// alone unless InstallOptions.Force is set. Those skills come back as
// ActionSkipped with a Reason, exactly as Uninstall reports them, and the
// error wraps ErrNeedsForce. One blocked destination does not stop the others
// from being written.
//
// The results are meaningful even when the error is not nil. They describe
// everything that happened before it.
func (in *Installer) Install(ctx context.Context, o InstallOptions) ([]InstallResult, error) {
	statuses, err := in.Status(ctx, o)
	if err != nil {
		return nil, err
	}

	var blocked []InstallStatus
	results := make([]InstallResult, 0, len(statuses))
	for _, st := range statuses {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		r := InstallResult{Skill: st.Skill, Target: st.Target, Path: st.Path, Before: st.State}
		switch {
		case st.State == StateModified && !o.Force:
			r.Action, r.Reason = ActionSkipped, "edited after installing; use --force"
			blocked = append(blocked, st)
		case st.State == StateForeign && !o.Force:
			r.Action, r.Reason = ActionSkipped, "installed by something else; use --force"
			blocked = append(blocked, st)
		case st.State == StateUpToDate && !o.Force:
			r.Action, r.Reason = ActionSkipped, "already up to date"
		case st.State == StateMissing:
			r.Action = ActionInstalled
		default:
			r.Action = ActionUpdated
		}
		if r.Action != ActionSkipped && !o.DryRun {
			if err := in.write(ctx, st.Skill, st.Path); err != nil {
				return results, fmt.Errorf("install %s into %s: %w", st.Skill.Name, st.Target.Dir, err)
			}
		}
		results = append(results, r)
	}
	if len(blocked) > 0 {
		return results, &ForceRequiredError{Blocked: blocked}
	}
	return results, nil
}

// Uninstall removes the selected skills from the resolved targets. Skills this
// tool did not install are left alone unless InstallOptions.Force is set.
//
// The results are meaningful even when the error is not nil.
func (in *Installer) Uninstall(ctx context.Context, o InstallOptions) ([]InstallResult, error) {
	statuses, err := in.Status(ctx, o)
	if err != nil {
		return nil, err
	}
	results := make([]InstallResult, 0, len(statuses))
	for _, st := range statuses {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		r := InstallResult{Skill: st.Skill, Target: st.Target, Path: st.Path, Before: st.State}
		switch {
		case st.State == StateMissing:
			r.Action, r.Reason = ActionSkipped, "not installed"
		case st.State == StateForeign && !o.Force:
			r.Action, r.Reason = ActionSkipped, "installed by something else; use --force"
		case st.State == StateModified && !o.Force:
			r.Action, r.Reason = ActionSkipped, "edited after installing; use --force"
		default:
			r.Action = ActionRemoved
			if !o.DryRun {
				if err := os.RemoveAll(st.Path); err != nil {
					return results, err
				}
			}
		}
		results = append(results, r)
	}
	return results, nil
}

// write materialises one skill at dest.
func (in *Installer) write(ctx context.Context, sk Skill, dest string) error {
	src, err := sk.FS()
	if err != nil {
		return err
	}
	return skillfs.Write(ctx, src, dest, skillfs.WriteOptions{
		Transform:  in.stamp(sk),
		Executable: in.executable,
	})
}

// stamp records who installed the skill, from what version, and what it held,
// in the manifest's frontmatter.
func (in *Installer) stamp(sk Skill) func(string, []byte) ([]byte, error) {
	if !in.metadata {
		return nil
	}
	return func(name string, data []byte) ([]byte, error) {
		if name != SkillFile {
			return data, nil
		}
		return manifest.With(data, []manifest.Entry{
			{Key: MetaKeyEmbeddedBy, Value: in.toolName},
			{Key: MetaKeyEmbeddedVersion, Value: in.version},
			{Key: MetaKeyEmbeddedAt, Value: in.now().UTC().Format(time.RFC3339)},
			{Key: MetaKeyEmbeddedDigest, Value: sk.Digest()},
		}), nil
	}
}
