package skillembed_test

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	skillembed "github.com/mpyw/go-skill-embed"
)

//go:embed testdata/skills
var installTestSkills embed.FS

// newInstaller returns an installer whose every scope lands inside a temporary
// directory, so a test can never write into the developer's real agent dirs.
func newInstaller(t *testing.T, opts ...skillembed.InstallerOption) (*skillembed.Installer, string) {
	t.Helper()
	root := t.TempDir()
	set := skillembed.MustSkillsFromFS(installTestSkills, "testdata/skills")
	base := []skillembed.InstallerOption{
		skillembed.WithToolName("testtool"),
		skillembed.WithVersion("v1.0.0"),
		skillembed.WithProjectRoot(root),
		skillembed.WithOutput(&bytes.Buffer{}),
	}
	return skillembed.NewInstaller(set, append(base, opts...)...), root
}

func TestSkillNameFallsBackToTheDirectory(t *testing.T) {
	set := skillembed.MustSkillsFromFS(installTestSkills, "testdata/skills")

	demo, ok := set.Lookup("demo-skill")
	if !ok {
		t.Fatal("demo-skill not found")
	}
	if !strings.HasPrefix(demo.Description, "A skill used by this module's tests") {
		t.Errorf("Description = %q", demo.Description)
	}

	// bare-skill has no frontmatter, so its name comes from the directory.
	bare, ok := set.Lookup("bare-skill")
	if !ok {
		t.Fatal("a manifest without frontmatter should still be discovered")
	}
	if bare.Description != "" {
		t.Errorf("Description = %q, want empty", bare.Description)
	}
}

func TestProjectScopeMergesTheSharedDirectory(t *testing.T) {
	in, root := newInstaller(t)

	targets, err := in.Targets(skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"all"}, Scope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	// Every agent but Claude Code reads .agents/skills at project scope.
	if len(targets) != 2 {
		for _, tg := range targets {
			t.Logf("target %s", tg.Label())
		}
		t.Fatalf("got %d project targets, want 2", len(targets))
	}
	want := map[string]int{
		filepath.Join(root, ".agents", "skills"): 5,
		filepath.Join(root, ".claude", "skills"): 1,
	}
	for _, tg := range targets {
		n, ok := want[tg.Dir]
		if !ok {
			t.Errorf("unexpected target %s", tg.Dir)
			continue
		}
		if len(tg.Agents) != n {
			t.Errorf("%s serves %d agents, want %d", tg.Dir, len(tg.Agents), n)
		}
	}
}

func TestInstallLifecycle(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")
	opts := skillembed.InstallOptions{Dir: dest, Names: []string{"demo-skill"}}

	results, err := in.Install(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Action != skillembed.ActionInstalled {
		t.Fatalf("first install = %+v", results)
	}

	manifest := filepath.Join(dest, "demo-skill", "SKILL.md")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Body text that must survive") {
		t.Errorf("body not copied:\n%s", body)
	}
	for _, key := range []string{skillembed.MetaKeyEmbeddedBy, skillembed.MetaKeyEmbeddedDigest} {
		if !strings.Contains(string(body), key+":") {
			t.Errorf("%s missing from the installed manifest:\n%s", key, body)
		}
	}

	// Installing again is a no-op.
	statuses, err := in.Status(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != skillembed.StateUpToDate {
		t.Fatalf("state after install = %s, want %s", statuses[0].State, skillembed.StateUpToDate)
	}
	results, err = in.Install(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Action != skillembed.ActionSkipped {
		t.Errorf("second install = %s, want %s", results[0].Action, skillembed.ActionSkipped)
	}

	// An edit is noticed, and is not overwritten without --force.
	if err := os.WriteFile(manifest, append(body, []byte("\nedited by hand\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	statuses, err = in.Status(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != skillembed.StateModified {
		t.Fatalf("state after an edit = %s, want %s", statuses[0].State, skillembed.StateModified)
	}
	if _, err := in.Install(ctx, opts); err == nil {
		t.Error("install overwrote an edited skill without --force")
	}

	forced := opts
	forced.Force = true
	results, err = in.Install(ctx, forced)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Action != skillembed.ActionUpdated {
		t.Errorf("forced install = %s, want %s", results[0].Action, skillembed.ActionUpdated)
	}

	removed, err := in.Uninstall(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if removed[0].Action != skillembed.ActionRemoved {
		t.Fatalf("uninstall = %+v", removed[0])
	}
	if _, err := os.Stat(filepath.Join(dest, "demo-skill")); err == nil {
		t.Error("the skill directory survived uninstall")
	}
}

func TestSkillsFromAnotherToolAreLeftAlone(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")
	opts := skillembed.InstallOptions{Dir: dest, Names: []string{"demo-skill"}}

	if _, err := in.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}

	// A second tool of a different name finds the same directory occupied.
	other, _ := newInstaller(t, skillembed.WithToolName("othertool"), skillembed.WithProjectRoot(root))
	statuses, err := other.Status(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != skillembed.StateForeign {
		t.Fatalf("state = %s, want %s", statuses[0].State, skillembed.StateForeign)
	}
	if _, err := other.Install(ctx, opts); err == nil {
		t.Error("install replaced another tool's skill without --force")
	}

	results, err := other.Uninstall(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Action != skillembed.ActionSkipped {
		t.Errorf("uninstall = %+v, want it skipped", results[0])
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")

	results, err := in.Install(ctx, skillembed.InstallOptions{Dir: dest, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("a dry run created the destination")
	}
}

func TestRun(t *testing.T) {
	ctx := t.Context()
	out := &bytes.Buffer{}
	in, root := newInstaller(t, skillembed.WithOutput(out))
	dest := filepath.Join(root, "skills")

	if err := in.Run(ctx, []string{"install", "--dir", dest, "demo-skill"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "installed") {
		t.Errorf("install said:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dest, "demo-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := in.Run(ctx, []string{"list", "--dir", dest}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "up-to-date") {
		t.Errorf("list said:\n%s", out)
	}

	if err := in.Run(ctx, []string{"nonsense"}); err == nil {
		t.Error("an unknown subcommand was accepted")
	}
	if err := in.Run(ctx, []string{"install", "--agent", "nonsense"}); err == nil {
		t.Error("an unknown agent was accepted")
	}
}

// A SKILL.md with no frontmatter gains one on install. It must still read as
// up-to-date afterwards, which it does not if the added block cannot be
// stripped back off for the digest.
func TestSkillWithoutFrontmatterInstallsClean(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)
	opts := skillembed.InstallOptions{Dir: filepath.Join(root, "skills"), Names: []string{"bare-skill"}}

	if _, err := in.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}
	statuses, err := in.Status(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != skillembed.StateUpToDate {
		t.Errorf("state after install = %s, want %s", statuses[0].State, skillembed.StateUpToDate)
	}
}

// The default is "detected". It keeps the agents already present, and falls
// back to every agent when there is nothing to go on.
func TestDefaultAgentDetects(t *testing.T) {
	in, root := newInstaller(t)
	agents := filepath.Join(root, ".agents", "skills")
	claude := filepath.Join(root, ".claude", "skills")

	// Nothing is there yet. At project scope the fallback is two directories,
	// because five of the six agents share one.
	targets, err := in.Targets(skillembed.InstallOptions{Scope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	served := map[string]int{}
	for _, tg := range targets {
		served[tg.Dir] = len(tg.Agents)
	}
	if want := map[string]int{agents: 5, claude: 1}; len(served) != len(want) {
		t.Fatalf("with nothing present, got %v, want %v", served, want)
	}
	if served[agents] != 5 || served[claude] != 1 {
		t.Errorf("got %v, want .agents/skills for 5 agents and .claude/skills for 1", served)
	}

	// A repository that already has .claude is a Claude Code repository.
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	targets, err = in.Targets(skillembed.InstallOptions{Scope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Dir != claude {
		t.Fatalf("got %d targets (%+v), want only .claude/skills", len(targets), targets)
	}
	if len(targets[0].Agents) != 1 || targets[0].Agents[0].Name != "claude-code" {
		t.Errorf("target serves %+v, want claude-code alone", targets[0].Agents)
	}

	// "all" ignores what is present.
	targets, err = in.Targets(skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"all"}, Scope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Errorf("all gave %d targets, want 2", len(targets))
	}
}

// An embedded .DS_Store was committed and ships to everyone, so it is an error
// rather than something to skip quietly.
func TestEmbeddedJunkIsRejected(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/demo/SKILL.md":  &fstest.MapFile{Data: []byte("---\nname: demo\n---\n")},
		"skills/demo/.DS_Store": &fstest.MapFile{Data: []byte("\x00\x01binary")},
	}

	_, err := skillembed.SkillsFromFS(fsys, "skills")
	if err == nil {
		t.Fatal("a skill holding .DS_Store was accepted")
	}
	for _, want := range []string{"skills/demo/.DS_Store", "all:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// The same file beside an installed skill is the file browser, not the user.
func TestInstalledJunkIsIgnored(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)
	opts := skillembed.InstallOptions{Dir: filepath.Join(root, "skills"), Names: []string{"demo-skill"}}

	if _, err := in.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}
	junk := filepath.Join(root, "skills", "demo-skill", ".DS_Store")
	if err := os.WriteFile(junk, []byte("\x00\x01binary"), 0o644); err != nil {
		t.Fatal(err)
	}

	statuses, err := in.Status(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != skillembed.StateUpToDate {
		t.Errorf("state = %s, want %s", statuses[0].State, skillembed.StateUpToDate)
	}
	results, err := in.Install(ctx, opts)
	if err != nil {
		t.Fatalf("install refused over a .DS_Store: %v", err)
	}
	if results[0].Action != skillembed.ActionSkipped {
		t.Errorf("install = %s, want %s", results[0].Action, skillembed.ActionSkipped)
	}
}

// A destination that has to be forced no longer stops the others. It comes
// back as ActionSkipped, the way Uninstall has always reported the same thing,
// and the error is one a caller can match.
func TestInstallReportsBlockedAndWritesTheRest(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")

	// Something else owns demo-skill's name.
	if err := os.MkdirAll(filepath.Join(dest, "demo-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "demo-skill", "SKILL.md"), []byte("hand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := in.Install(ctx, skillembed.InstallOptions{Dir: dest})
	if !errors.Is(err, skillembed.ErrNeedsForce) {
		t.Fatalf("err = %v, want it to wrap ErrNeedsForce", err)
	}
	var blocked *skillembed.ForceRequiredError
	if !errors.As(err, &blocked) {
		t.Fatalf("err = %T, want *ForceRequiredError", err)
	}
	if len(blocked.Blocked) != 1 || blocked.Blocked[0].Skill.Name != "demo-skill" {
		t.Errorf("blocked = %+v, want demo-skill alone", blocked.Blocked)
	}

	byName := map[string]skillembed.InstallResult{}
	for _, r := range results {
		byName[r.Skill.Name] = r
	}
	if len(byName) != 2 {
		t.Fatalf("results cover %d skills, want 2: %+v", len(byName), results)
	}
	if got := byName["demo-skill"].Action; got != skillembed.ActionSkipped {
		t.Errorf("demo-skill = %s, want %s", got, skillembed.ActionSkipped)
	}
	if got := byName["bare-skill"].Action; got != skillembed.ActionInstalled {
		t.Errorf("bare-skill = %s, want %s", got, skillembed.ActionInstalled)
	}
	if _, err := os.Stat(filepath.Join(dest, "bare-skill", "SKILL.md")); err != nil {
		t.Errorf("the unblocked skill was not written: %v", err)
	}
	// The hand written manifest is untouched.
	body, err := os.ReadFile(filepath.Join(dest, "demo-skill", "SKILL.md"))
	if err != nil || string(body) != "hand written\n" {
		t.Errorf("the blocked skill was overwritten: %q %v", body, err)
	}
}

// Anything in an installed directory that cannot be hashed, such as a symlink
// a user dropped in, used to fail Status and with it Install and Uninstall for
// every skill at that target, --force included. The only way out was rm -rf.
func TestUnreadableInstallStaysRepairable(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")
	opts := skillembed.InstallOptions{Dir: dest}

	if _, err := in.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dest, "demo-skill", "link.txt")
	if err := os.Symlink(filepath.Join(root, "elsewhere"), link); err != nil {
		t.Fatal(err)
	}

	statuses, err := in.Status(ctx, opts)
	if err != nil {
		t.Fatalf("Status failed over a symlink: %v", err)
	}
	byName := map[string]skillembed.State{}
	for _, st := range statuses {
		byName[st.Skill.Name] = st.State
	}
	if byName["demo-skill"] != skillembed.StateForeign {
		t.Errorf("demo-skill = %s, want %s", byName["demo-skill"], skillembed.StateForeign)
	}
	if byName["bare-skill"] != skillembed.StateUpToDate {
		t.Errorf("the unrelated skill was dragged in: bare-skill = %s", byName["bare-skill"])
	}

	forced := opts
	forced.Force = true
	results, err := in.Install(ctx, forced)
	if err != nil {
		t.Fatalf("forced install failed over a symlink: %v", err)
	}
	for _, r := range results {
		if r.Skill.Name == "demo-skill" && r.Action != skillembed.ActionUpdated {
			t.Errorf("forced install = %s, want %s", r.Action, skillembed.ActionUpdated)
		}
	}
	if _, err := os.Lstat(link); err == nil {
		t.Error("the symlink survived a forced install")
	}

	if _, err := in.Uninstall(ctx, forced); err != nil {
		t.Fatalf("forced uninstall failed: %v", err)
	}
}

// A front end wants to map "you typed a bad value" to a different exit code
// than "the disk is full", and so does a test.
func TestErrorsAreMatchable(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")

	for _, c := range []struct {
		name string
		opts skillembed.InstallOptions
		want error
	}{
		{"unknown agent", skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"nonsense"}}, skillembed.ErrUnknownAgent},
		{"unknown scope", skillembed.InstallOptions{Scope: "nonsense"}, skillembed.ErrUnknownScope},
		{"unknown skill", skillembed.InstallOptions{Dir: dest, Names: []string{"nonsense"}}, skillembed.ErrUnknownSkill},
		{"nothing selected", skillembed.InstallOptions{Agents: []skillembed.AgentSelector{""}}, skillembed.ErrNoAgentSelected},
		// Dir wins over agent and scope, but a bad value beside it is still a
		// mistake, and saying nothing about it was the old behaviour.
		{"dir with a bad scope", skillembed.InstallOptions{Dir: dest, Scope: "nonsense"}, skillembed.ErrUnknownScope},
		{"dir with a bad agent", skillembed.InstallOptions{Dir: dest, Agents: []skillembed.AgentSelector{"nonsense"}}, skillembed.ErrUnknownAgent},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := in.Status(ctx, c.opts)
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want it to wrap %v", err, c.want)
			}
		})
	}
}

// A script that loses its executable bit cannot be run by the agent. Nobody
// edited it, so repairing it must not need --force.
func TestLostExecutableBitIsRepaired(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")
	opts := skillembed.InstallOptions{Dir: dest, Names: []string{"demo-skill"}}

	if _, err := in.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dest, "demo-skill", "scripts", "run.sh")
	info, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("the script was installed without the executable bit: %v", info.Mode())
	}

	if err := os.Chmod(script, 0o644); err != nil {
		t.Fatal(err)
	}
	statuses, err := in.Status(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != skillembed.StateOutdated {
		t.Errorf("state = %s, want %s", statuses[0].State, skillembed.StateOutdated)
	}

	results, err := in.Install(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Action != skillembed.ActionUpdated {
		t.Errorf("install = %s, want %s", results[0].Action, skillembed.ActionUpdated)
	}
	if info, err := os.Stat(script); err != nil || info.Mode()&0o111 == 0 {
		t.Errorf("the executable bit was not restored: %v %v", info.Mode(), err)
	}
}

// Two skills whose names differ only in case install over each other on a case
// insensitive file system, and every run flips the directory's contents while
// reporting success.
func TestNamesDifferingOnlyInCaseAreRejected(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/a/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: Demo\n---\nA\n")},
		"skills/b/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\n---\nB\n")},
	}
	_, err := skillembed.SkillsFromFS(fsys, "skills")
	if err == nil {
		t.Fatal("the set was accepted")
	}
	if !strings.Contains(err.Error(), "differ only in case") {
		t.Errorf("error does not explain the collision: %v", err)
	}
}

// installerFromFS builds an installer over a crafted file system, so that a
// test can describe the skill it needs instead of adding a directory to
// testdata that every other test then has to carry.
func installerFromFS(t *testing.T, fsys fs.FS, root string, opts ...skillembed.InstallerOption) *skillembed.Installer {
	t.Helper()
	base := []skillembed.InstallerOption{
		skillembed.WithToolName("testtool"),
		skillembed.WithVersion("v1.0.0"),
		skillembed.WithProjectRoot(t.TempDir()),
		skillembed.WithOutput(&bytes.Buffer{}),
	}
	return skillembed.NewInstaller(skillembed.MustSkillsFromFS(fsys, root), append(base, opts...)...)
}

// Uninstall is the one destructive operation, and no front end test drove it.
// The copy at project scope is the decoy: an uninstall that ignored --dir, or
// bound it to the wrong option, deletes that one instead of the directory the
// user named.
func TestRunUninstallDeletesOnlyTheDirectoryNamed(t *testing.T) {
	ctx := t.Context()
	out := &bytes.Buffer{}
	in, root := newInstaller(t, skillembed.WithOutput(out))
	dest := filepath.Join(root, "custom")
	decoy := filepath.Join(root, ".claude", "skills", "demo-skill")

	if err := in.Run(ctx, []string{"install", "--dir", dest, "demo-skill"}); err != nil {
		t.Fatal(err)
	}
	if err := in.Run(ctx, []string{"install", "--agent", "claude-code", "demo-skill"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(decoy); err != nil {
		t.Fatalf("the project scope copy was never written, so the test proves nothing: %v", err)
	}

	out.Reset()
	if err := in.Run(ctx, []string{"uninstall", "--dir", dest, "demo-skill"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "removed") {
		t.Errorf("uninstall said:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dest, "demo-skill")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the skill survived uninstall --dir: %v", err)
	}
	if _, err := os.Stat(decoy); err != nil {
		t.Errorf("uninstall --dir deleted from the agent directory instead: %v", err)
	}

	// Removing what is no longer there is a skip, not a failure.
	out.Reset()
	if err := in.Run(ctx, []string{"uninstall", "--dir", dest, "demo-skill"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "not installed") {
		t.Errorf("a second uninstall said:\n%s", out)
	}
}

// `remove` and `ls` are documented aliases. They have to reach the same code,
// not a copy of it that drifts.
func TestRunAliasesMatchTheirCommands(t *testing.T) {
	ctx := t.Context()
	out := &bytes.Buffer{}
	in, root := newInstaller(t, skillembed.WithOutput(out))
	dest := filepath.Join(root, "custom")

	say := func(args ...string) string {
		t.Helper()
		out.Reset()
		if err := in.Run(ctx, args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out.String()
	}

	say("install", "--dir", dest)
	if list, ls := say("list", "--dir", dest), say("ls", "--dir", dest); list != ls {
		t.Errorf("ls said:\n%s\nlist said:\n%s", ls, list)
	}

	removed := say("remove", "--dir", dest)
	if !strings.Contains(removed, "removed") {
		t.Errorf("remove said:\n%s", removed)
	}
	if _, err := os.Stat(filepath.Join(dest, "demo-skill")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("remove left the skill behind: %v", err)
	}

	say("install", "--dir", dest)
	if uninstalled := say("uninstall", "--dir", dest); removed != uninstalled {
		t.Errorf("remove said:\n%s\nuninstall said:\n%s", removed, uninstalled)
	}
}

// --scope user has to land in the user's own directory and nowhere near the
// project. HOME is a temporary directory here, so the test cannot reach the
// developer's real ~/.claude even if the code does the wrong thing.
func TestRunInstallsAtUserScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	ctx := t.Context()
	out := &bytes.Buffer{}
	in, root := newInstaller(t, skillembed.WithOutput(out), skillembed.WithAgents(skillembed.AgentClaudeCode))

	if err := in.Run(ctx, []string{"install", "--scope", "user", "demo-skill"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "demo-skill", skillembed.SkillFile)); err != nil {
		t.Fatalf("--scope user did not install into the home directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("--scope user also wrote into the project: %v", err)
	}

	// A scope that is neither is reported rather than guessed at. The flag
	// package renders the cause with %v, so only the text can be matched.
	err := in.Run(ctx, []string{"install", "--scope", "nonsense", "demo-skill"})
	if err == nil {
		t.Fatal("--scope nonsense was accepted")
	}
	if !strings.Contains(err.Error(), "unknown scope") {
		t.Errorf("err = %v, want it to name the unknown scope", err)
	}
}

// WithMetadata(false) carries the README's WARNING. Without the frontmatter
// there is nothing to compare against, so a skill this tool installed a moment
// ago reads as foreign and every later install needs --force.
func TestInstallWithoutMetadataReadsAsForeign(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t, skillembed.WithMetadata(false))
	dest := filepath.Join(root, "skills")
	opts := skillembed.InstallOptions{Dir: dest, Names: []string{"demo-skill"}}

	if _, err := in.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dest, "demo-skill", skillembed.SkillFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		skillembed.MetaKeyEmbeddedBy,
		skillembed.MetaKeyEmbeddedVersion,
		skillembed.MetaKeyEmbeddedAt,
		skillembed.MetaKeyEmbeddedDigest,
	} {
		if strings.Contains(string(body), key) {
			t.Errorf("%s was written although metadata is off:\n%s", key, body)
		}
	}
	// The skill itself still arrives intact.
	if !strings.Contains(string(body), "Body text that must survive") {
		t.Errorf("the skill was not copied:\n%s", body)
	}

	statuses, err := in.Status(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != skillembed.StateForeign {
		t.Errorf("state = %s, want %s: without metadata nothing can be recognised",
			statuses[0].State, skillembed.StateForeign)
	}
	if _, err := in.Install(ctx, opts); !errors.Is(err, skillembed.ErrNeedsForce) {
		t.Errorf("a second install = %v, want it to need --force", err)
	}
	forced := opts
	forced.Force = true
	results, err := in.Install(ctx, forced)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Action != skillembed.ActionUpdated {
		t.Errorf("forced install = %s, want %s", results[0].Action, skillembed.ActionUpdated)
	}
}

// WithExecutable carries the README's CAUTION: embed.FS drops file modes, so
// the rule is the only thing that decides them. It replaces the shebang
// default rather than adding to it, which is visible in both directions, and
// the same rule decides whether an installation is still intact.
func TestInstallExecutableRuleDecidesTheMode(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/demo/SKILL.md":       &fstest.MapFile{Data: []byte("---\nname: demo\n---\n\n# Demo\n")},
		"skills/demo/bin/tool":       &fstest.MapFile{Data: []byte("a program with no shebang\n")},
		"skills/demo/scripts/run.sh": &fstest.MapFile{Data: []byte("#!/bin/sh\necho hi\n")},
	}
	rule := func(name string, _ []byte) bool { return name == "bin/tool" }

	for _, c := range []struct {
		name string
		opts []skillembed.InstallerOption
		want map[string]bool
	}{
		{"the default marks a shebang", nil,
			map[string]bool{"bin/tool": false, "scripts/run.sh": true}},
		{"a rule replaces the default", []skillembed.InstallerOption{skillembed.WithExecutable(rule)},
			map[string]bool{"bin/tool": true, "scripts/run.sh": false}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := t.Context()
			in := installerFromFS(t, fsys, "skills", c.opts...)
			dest := filepath.Join(t.TempDir(), "skills")
			opts := skillembed.InstallOptions{Dir: dest}

			if _, err := in.Install(ctx, opts); err != nil {
				t.Fatal(err)
			}
			for p, want := range c.want {
				full := filepath.Join(dest, "demo", filepath.FromSlash(p))
				info, err := os.Stat(full)
				if err != nil {
					t.Fatal(err)
				}
				if got := info.Mode()&0o111 != 0; got != want {
					t.Errorf("%s mode = %v, executable = %v, want %v", p, info.Mode(), got, want)
				}
			}

			statuses, err := in.Status(ctx, opts)
			if err != nil {
				t.Fatal(err)
			}
			if statuses[0].State != skillembed.StateUpToDate {
				t.Fatalf("state after install = %s, want %s", statuses[0].State, skillembed.StateUpToDate)
			}

			// Flipping any file's bit away from what the rule asks for is
			// noticed, and it is outdated rather than modified because nobody
			// edited the contents.
			for p, want := range c.want {
				full := filepath.Join(dest, "demo", filepath.FromSlash(p))
				flipped, restored := os.FileMode(0o755), os.FileMode(0o644)
				if want {
					flipped, restored = 0o644, 0o755
				}
				if err := os.Chmod(full, flipped); err != nil {
					t.Fatal(err)
				}
				statuses, err := in.Status(ctx, opts)
				if err != nil {
					t.Fatal(err)
				}
				if statuses[0].State != skillembed.StateOutdated {
					t.Errorf("%s at mode %v: state = %s, want %s", p, flipped, statuses[0].State, skillembed.StateOutdated)
				}
				if err := os.Chmod(full, restored); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// Label is what every listing prints. A target built from --dir serves no
// agent, and naming agents there would claim the skills went somewhere they
// did not.
func TestInstallTargetLabel(t *testing.T) {
	in, root := newInstaller(t)

	custom := filepath.Join(root, "custom")
	targets, err := in.Targets(skillembed.InstallOptions{Dir: custom})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("--dir gave %d targets, want 1", len(targets))
	}
	if got := targets[0].Label(); got != custom {
		t.Errorf("Label() = %q, want %q", got, custom)
	}

	targets, err = in.Targets(skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"claude-code", "codex"}, Scope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		filepath.Join(root, ".claude", "skills"): filepath.Join(root, ".claude", "skills") + " (Claude Code)",
		filepath.Join(root, ".agents", "skills"): filepath.Join(root, ".agents", "skills") + " (Codex)",
	}
	if len(targets) != len(want) {
		t.Fatalf("got %d targets, want %d", len(targets), len(want))
	}
	for _, tg := range targets {
		if got := tg.Label(); got != want[tg.Dir] {
			t.Errorf("Label() = %q, want %q", got, want[tg.Dir])
		}
	}
}

// WithAgents is the documented way to restrict what a tool offers. An agent
// left out has to disappear from the help, from "all", from "detected" and
// from the values --agent accepts, not just from one of them.
func TestWithAgentsRestrictsWhatIsOffered(t *testing.T) {
	in, root := newInstaller(t, skillembed.WithAgents(skillembed.AgentClaudeCode, skillembed.AgentCodex))
	claude := filepath.Join(root, ".claude", "skills")
	shared := filepath.Join(root, ".agents", "skills")

	if got, want := in.AgentChoices(), "{claude-code|codex}"; got != want {
		t.Errorf("AgentChoices() = %q, want %q", got, want)
	}

	// "all" and the "detected" fallback both mean the two that are offered.
	for _, o := range []skillembed.InstallOptions{
		{Agents: []skillembed.AgentSelector{"all"}, Scope: "project"},
		{Scope: "project"},
	} {
		targets, err := in.Targets(o)
		if err != nil {
			t.Fatal(err)
		}
		dirs := map[string]bool{}
		for _, tg := range targets {
			dirs[tg.Dir] = true
		}
		if len(dirs) != 2 || !dirs[claude] || !dirs[shared] {
			t.Errorf("Targets(%+v) = %v, want %s and %s", o, dirs, claude, shared)
		}
	}

	// An agent that was left out is no longer a value --agent accepts, and the
	// complaint lists only what is on offer.
	_, err := in.Targets(skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"cursor"}, Scope: "project"})
	if !errors.Is(err, skillembed.ErrUnknownAgent) {
		t.Fatalf("err = %v, want it to wrap ErrUnknownAgent", err)
	}
	if !strings.Contains(err.Error(), "want one of claude-code, codex") {
		t.Errorf("err = %v, want it to offer only the restricted agents", err)
	}
}

// WithDefaultAgents is the documented way to match `gh skill install`, whose
// default is github-copilot alone. It replaces "detected", so a project that
// already has .claude must not pull Claude Code back in.
func TestWithDefaultAgentsReplacesDetection(t *testing.T) {
	in, root := newInstaller(t, skillembed.WithDefaultAgents("github-copilot"))
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	targets, err := in.Targets(skillembed.InstallOptions{Scope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Dir != filepath.Join(root, ".agents", "skills") {
		t.Fatalf("targets = %+v, want .agents/skills alone", targets)
	}
	if len(targets[0].Agents) != 1 || targets[0].Agents[0].Name != "github-copilot" {
		t.Errorf("target serves %+v, want github-copilot alone", targets[0].Agents)
	}

	// --agent still wins over the default.
	targets, err = in.Targets(skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"claude-code"}, Scope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Dir != filepath.Join(root, ".claude", "skills") {
		t.Errorf("targets = %+v, want .claude/skills", targets)
	}
}

// WithDefaultScope decides where a run with no --scope writes. HOME is a
// temporary directory here, so a mistake cannot reach the real one.
func TestWithDefaultScopeChangesWhereSkillsLand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	in, root := newInstaller(t,
		skillembed.WithDefaultScope(skillembed.ScopeUser),
		skillembed.WithAgents(skillembed.AgentClaudeCode))

	if got := in.DefaultScope(); got != skillembed.ScopeUser {
		t.Errorf("DefaultScope() = %q, want %q", got, skillembed.ScopeUser)
	}
	targets, err := in.Targets(skillembed.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".claude", "skills"); len(targets) != 1 || targets[0].Dir != want {
		t.Fatalf("targets = %+v, want %s", targets, want)
	}

	// An explicit scope still wins over the default.
	targets, err = in.Targets(skillembed.InstallOptions{Scope: skillembed.ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, ".claude", "skills"); len(targets) != 1 || targets[0].Dir != want {
		t.Errorf("targets = %+v, want %s", targets, want)
	}
}

// NeedsForce is the predicate a front end branches on to decide whether to
// suggest --force. Answering yes for an outdated copy tells a user to force
// what is simply out of date; answering no for a modified one loses their
// edits. It has to agree with the states Install refuses to write over.
func TestStateNeedsForce(t *testing.T) {
	for _, c := range []struct {
		state skillembed.State
		want  bool
	}{
		{skillembed.StateMissing, false},
		{skillembed.StateUpToDate, false},
		{skillembed.StateOutdated, false},
		{skillembed.StateModified, true},
		{skillembed.StateForeign, true},
	} {
		if got := c.state.NeedsForce(); got != c.want {
			t.Errorf("%s.NeedsForce() = %v, want %v", c.state, got, c.want)
		}
	}
}

// The message is all the user is left with when install refuses. It has to
// name every destination, say what was found there, and say what to do next.
func TestForceRequiredErrorMessage(t *testing.T) {
	err := &skillembed.ForceRequiredError{Blocked: []skillembed.InstallStatus{
		{Path: filepath.Join("one", "demo-skill"), State: skillembed.StateModified},
		{Path: filepath.Join("two", "other-skill"), State: skillembed.StateForeign},
	}}

	msg := err.Error()
	for _, want := range []string{
		filepath.Join("one", "demo-skill"),
		string(skillembed.StateModified),
		filepath.Join("two", "other-skill"),
		string(skillembed.StateForeign),
		"--force",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the message does not mention %q:\n%s", want, msg)
		}
	}
	if !errors.Is(err, skillembed.ErrNeedsForce) {
		t.Error("the error does not unwrap to ErrNeedsForce")
	}
}

// A skill name becomes a directory under the destination, so it is the one
// piece of a crafted SKILL.md that could write outside it, or hide the skill
// from the agent meant to read it. Everything here is refused while the set is
// read, which is before anything could be written.
func TestSkillNameRejections(t *testing.T) {
	for _, c := range []struct {
		name     string
		dir      string
		manifest string
		want     string
	}{
		{name: "parent directory", manifest: "---\nname: ../evil\n---\n", want: "must not contain a path separator"},
		{name: "windows separator", manifest: "---\nname: ..\\evil\n---\n", want: "must not contain a path separator"},
		{name: "nested", manifest: "---\nname: a/b\n---\n", want: "must not contain a path separator"},
		{name: "hidden", manifest: "---\nname: .hidden\n---\n", want: "must not start with a dot"},
		{name: "dot", manifest: "---\nname: .\n---\n", want: `invalid skill name "."`},
		{name: "dot dot", manifest: "---\nname: ..\n---\n", want: `invalid skill name ".."`},
		// The name falls back to the directory, so the directory is crafted too.
		{name: "hidden directory", dir: ".hidden", manifest: "no frontmatter at all\n", want: "must not start with a dot"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := c.dir
			if dir == "" {
				dir = "crafted"
			}
			fsys := fstest.MapFS{
				"skills/" + dir + "/SKILL.md": &fstest.MapFile{Data: []byte(c.manifest)},
			}
			_, err := skillembed.SkillsFromFS(fsys, "skills")
			if err == nil {
				t.Fatalf("%q was accepted", c.manifest)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to say %q", err, c.want)
			}
			if !strings.Contains(err.Error(), "skills/"+dir) {
				t.Errorf("err = %v, want it to name the skill it came from", err)
			}
		})
	}

	// A dot inside a name is not a dot at the front of one, and refusing it
	// would be as wrong as accepting an escape.
	fsys := fstest.MapFS{
		"skills/ok/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo.v2\n---\n")},
		// An empty name is no name, so the directory decides, exactly as a
		// manifest with no name field does.
		"skills/fallback/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: \"\"\n---\n")},
	}
	set, err := skillembed.SkillsFromFS(fsys, "skills")
	if err != nil {
		t.Fatalf("a name holding a dot was refused: %v", err)
	}
	for _, want := range []string{"demo.v2", "fallback"} {
		if _, ok := set.Lookup(want); !ok {
			t.Errorf("names = %v, want %s", set.Names(), want)
		}
	}

	// A skill at the root of the file system has no directory name to fall
	// back to, so it has to declare one. Without this the name would be ".",
	// and the skill would be installed over its own destination directory.
	_, err = skillembed.SkillsFromFS(fstest.MapFS{
		"SKILL.md": &fstest.MapFile{Data: []byte("no frontmatter, so no name\n")},
	}, "")
	if err == nil {
		t.Fatal("a root manifest with no name was accepted")
	}
	if !strings.Contains(err.Error(), `invalid skill name "."`) {
		t.Errorf("err = %v, want it to refuse the name \".\"", err)
	}
}

// Two directories declaring one name install over each other, and every run
// would flip the contents while reporting success.
func TestSkillsFromFSRejectsDuplicateNames(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/first/SKILL.md":  &fstest.MapFile{Data: []byte("---\nname: demo\n---\nA\n")},
		"skills/second/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: demo\n---\nB\n")},
	}
	_, err := skillembed.SkillsFromFS(fsys, "skills")
	if err == nil {
		t.Fatal("two skills of one name were accepted")
	}
	for _, want := range []string{"skills/first", "skills/second", `"demo"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to mention %q", err, want)
		}
	}
}

// An embed that reached no skill is a build-time mistake, and a set of nothing
// would install nothing while reporting success.
func TestSkillsFromFSNeedsASkill(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/notes.md":      &fstest.MapFile{Data: []byte("not a skill\n")},
		"skills/sub/README.md": &fstest.MapFile{Data: []byte("still not a skill\n")},
		"elsewhere/x/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: x\n---\n")},
	}
	_, err := skillembed.SkillsFromFS(fsys, "skills")
	if err == nil {
		t.Fatal("a root holding no skill was accepted")
	}
	if !strings.Contains(err.Error(), "no SKILL.md found under skills") {
		t.Errorf("err = %v, want it to say nothing was found", err)
	}

	// A root that is not there at all says which root it could not read.
	_, err = skillembed.SkillsFromFS(fsys, "missing")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want it to wrap fs.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("err = %v, want it to name the root", err)
	}
}

// A root that holds a SKILL.md of its own is one skill, whatever else the tree
// holds. Reading it as a directory of skills instead would install its
// subdirectories as separate skills and lose the manifest at the top.
func TestSkillsFromFSReadsARootManifestAsOneSkill(t *testing.T) {
	ctx := t.Context()
	fsys := fstest.MapFS{
		"SKILL.md":          &fstest.MapFile{Data: []byte("---\nname: solo\ndescription: The whole tree.\n---\n\n# Solo\n")},
		"reference/tips.md": &fstest.MapFile{Data: []byte("tips\n")},
		// A nested manifest is part of the one skill, not a skill of its own.
		"nested/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: nested\n---\n")},
	}

	// An empty root means the top of the file system.
	set, err := skillembed.SkillsFromFS(fsys, "")
	if err != nil {
		t.Fatal(err)
	}
	if set.Len() != 1 {
		t.Fatalf("names = %v, want solo alone", set.Names())
	}
	sk, ok := set.Lookup("solo")
	if !ok {
		t.Fatalf("names = %v, want solo", set.Names())
	}
	if sk.Dir != "." {
		t.Errorf("Dir = %q, want %q", sk.Dir, ".")
	}
	if sk.Description != "The whole tree." {
		t.Errorf("Description = %q", sk.Description)
	}

	in := installerFromFS(t, fsys, "")
	dest := filepath.Join(t.TempDir(), "skills")
	opts := skillembed.InstallOptions{Dir: dest}
	if _, err := in.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"SKILL.md", "reference/tips.md", "nested/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dest, "solo", filepath.FromSlash(p))); err != nil {
			t.Errorf("%s did not come along: %v", p, err)
		}
	}
	statuses, err := in.Status(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != skillembed.StateUpToDate {
		t.Errorf("state after install = %s, want %s", statuses[0].State, skillembed.StateUpToDate)
	}
}

// MustSkillsFromFS is written at package level, where there is nobody to
// return an error to. Returning nil instead of panicking would leave a tool
// with a set of nothing, and every install would report success and write
// nothing at all.
func TestMustSkillsFromFSPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("a broken embed did not panic")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("recovered %T, want an error", r)
		}
		if !strings.Contains(err.Error(), "no SKILL.md found") {
			t.Errorf("panic = %v, want it to say what is wrong", err)
		}
	}()
	skillembed.MustSkillsFromFS(fstest.MapFS{"skills/notes.md": &fstest.MapFile{}}, "skills")
}

// A set that was never built is not a reason to crash inside somebody else's
// tool. The accessors answer for a nil set, so a mistake reaches the user as
// an empty listing rather than a panic in their main.
func TestSkillSetWithoutSkills(t *testing.T) {
	var set *skillembed.SkillSet

	if n := set.Len(); n != 0 {
		t.Errorf("Len() = %d, want 0", n)
	}
	if got := set.Skills(); got != nil {
		t.Errorf("Skills() = %v, want nil", got)
	}
	if got := set.Names(); len(got) != 0 {
		t.Errorf("Names() = %v, want none", got)
	}
	if _, ok := set.Lookup("demo-skill"); ok {
		t.Error("Lookup found a skill in a set that holds none")
	}

	in := skillembed.NewInstaller(set, skillembed.WithToolName("testtool"), skillembed.WithProjectRoot(t.TempDir()))
	if hint := in.UsageHint(); !strings.Contains(hint, "0 agent skills") {
		t.Errorf("UsageHint() = %q, want it to say there are none", hint)
	}
	results, err := in.Install(t.Context(), skillembed.InstallOptions{Dir: filepath.Join(t.TempDir(), "skills")})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("Install wrote %+v from a set that holds nothing", results)
	}
}
