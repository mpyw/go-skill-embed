package skillembed_test

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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

// A run resolves the project by searching, so a run from a subdirectory can
// land somewhere the reader did not expect. The output names the decision
// rather than leaving it to be inferred from a leaf path.
func TestOutputNamesTheProjectRoot(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)

	statuses, err := in.Status(ctx, skillembed.InstallOptions{Scope: skillembed.ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if got := statuses[0].Target.Root; got != root {
		t.Errorf("Root = %q, want %q", got, root)
	}
	if !strings.Contains(skillembed.RenderCLIStatus(statuses), "Project root: "+root) {
		t.Errorf("the listing does not name the root:\n%s", skillembed.RenderCLIStatus(statuses))
	}

	// A named directory is the destination outright, and user scope has no
	// project, so neither carries one.
	for _, o := range []skillembed.InstallOptions{
		{Dir: filepath.Join(root, "skills")},
		{Scope: skillembed.ScopeUser},
	} {
		t.Setenv("HOME", t.TempDir())
		statuses, err := in.Status(ctx, o)
		if err != nil {
			t.Fatal(err)
		}
		if got := statuses[0].Target.Root; got != "" {
			t.Errorf("%+v: Root = %q, want empty", o, got)
		}
		if strings.Contains(skillembed.RenderCLIStatus(statuses), "Project root:") {
			t.Errorf("%+v: the listing names a root it does not have", o)
		}
	}
}

// installCancelOnce is live until a path appears, and cancelled after that.
//
// A real cancellation lands wherever it lands, and the checks inside Install
// and Uninstall are only reached once Status has passed. Counting calls would
// work too, and would encode how many files each skill holds.
type installCancelOnce struct {
	context.Context
	written string
}

func (c installCancelOnce) Err() error {
	if _, err := os.Stat(c.written); err == nil {
		return context.Canceled
	}
	return nil
}

// A cancelled context stops the work between skills, and the results describe
// what happened before it. Without the checks, a cancelled run installs
// everything and reports success.
func TestCancellationStopsBetweenSkills(t *testing.T) {
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")
	opts := skillembed.InstallOptions{Dir: dest}

	t.Run("already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		statuses, err := in.Status(ctx, opts)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Status = %v, want context.Canceled", err)
		}
		if len(statuses) != 0 {
			t.Errorf("Status returned %d rows for a run that did nothing", len(statuses))
		}
		if _, err := in.Install(ctx, opts); !errors.Is(err, context.Canceled) {
			t.Errorf("Install = %v, want context.Canceled", err)
		}
		if _, err := os.Stat(dest); err == nil {
			t.Error("a cancelled install wrote something")
		}
	})

	// The skills are installed in name order, and the first one appears at its
	// destination only when its write has finished. Cancelling on that puts
	// the cancellation in Install's own loop, before the second skill.
	t.Run("cancelled inside the run", func(t *testing.T) {
		ctx := installCancelOnce{
			Context: t.Context(),
			written: filepath.Join(dest, "bare-skill"),
		}

		results, err := in.Install(ctx, opts)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Install = %v, want context.Canceled", err)
		}
		if len(results) != 1 {
			t.Fatalf("got %d results, want the one skill that was written", len(results))
		}
		if results[0].Action != skillembed.ActionInstalled {
			t.Errorf("result = %s, want %s", results[0].Action, skillembed.ActionInstalled)
		}
		if _, err := os.Stat(filepath.Join(dest, results[0].Skill.Name, "SKILL.md")); err != nil {
			t.Errorf("the reported skill is not on disk: %v", err)
		}
	})
}

// --agent is repeatable and one value may be a comma separated list. The help
// says "(repeatable)" and the README says both, and neither form was tested.
func TestAgentSelectorsSplitAndRepeat(t *testing.T) {
	ctx := t.Context()
	out := &bytes.Buffer{}
	in, root := newInstaller(t, skillembed.WithOutput(out))
	dest := filepath.Join(root, "skills")

	want := []string{"claude-code", "cursor"}
	for _, args := range [][]string{
		{"install", "--dry-run", "--dir", dest, "--agent", "claude-code,cursor"},
		{"install", "--dry-run", "--dir", dest, "--agent", "claude-code", "--agent", "cursor"},
		{"install", "--dry-run", "--dir", dest, "--agent", " claude-code , cursor "},
	} {
		out.Reset()
		if err := in.Run(ctx, args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}

	// The same two agents, whichever way they were written.
	for _, o := range []skillembed.InstallOptions{
		{Agents: []skillembed.AgentSelector{"claude-code,cursor"}},
		{Agents: []skillembed.AgentSelector{"claude-code", "cursor"}},
	} {
		targets, err := in.Targets(o)
		if err != nil {
			t.Fatalf("%+v: %v", o, err)
		}
		var named []string
		for _, tg := range targets {
			for _, a := range tg.Agents {
				named = append(named, a.Name)
			}
		}
		sort.Strings(named)
		if strings.Join(named, ",") != strings.Join(want, ",") {
			t.Errorf("%+v resolved %v, want %v", o, named, want)
		}
	}
}

// A project install writes where the project's own tree says, so a symbolic
// link committed at .claude/skills, or at any directory above it, used to aim
// the write and a later forced removal anywhere on the disk. Git stores a link
// as mode 120000, so cloning a repository and running the tool once was the
// whole of it.
func TestProjectInstallStaysInsideTheProject(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)

	outside := t.TempDir()
	kept := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(kept, []byte("not the tool's"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".claude", "skills")); err != nil {
		t.Fatal(err)
	}

	opts := skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"claude-code"}}
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"Targets", func() error { _, err := in.Targets(opts); return err }},
		{"Status", func() error { _, err := in.Status(ctx, opts); return err }},
		{"Install", func() error { _, err := in.Install(ctx, opts); return err }},
		{"Uninstall", func() error {
			forced := opts
			forced.Force = true
			_, err := in.Uninstall(ctx, forced)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, skillembed.ErrProjectEscapes) {
				t.Errorf("%s = %v, want ErrProjectEscapes", tc.name, err)
			}
		})
	}

	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "keep.txt" {
		t.Errorf("the directory outside the project was touched: %v", entries)
	}
}

// The bound is the project, not symbolic links. A repository that keeps its
// skills somewhere else in its own tree and links to them is doing nothing
// wrong, and user scope is the user's own directory to move.
func TestInstallFollowsLinksThatStayInReach(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)

	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, filepath.Join(root, ".claude", "skills")); err != nil {
		t.Fatal(err)
	}

	opts := skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"claude-code"}}
	if _, err := in.Install(ctx, opts); err != nil {
		t.Fatalf("Install = %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(shared, "demo-skill", "SKILL.md")); err != nil {
		t.Errorf("the skill did not land through the link: %v", err)
	}

	// --dir names the destination outright, which is the way out the refusal
	// points at, so it is not bounded by the project either.
	outside := t.TempDir()
	if _, err := in.Install(ctx, skillembed.InstallOptions{Dir: outside}); err != nil {
		t.Fatalf("Install --dir = %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "demo-skill", "SKILL.md")); err != nil {
		t.Errorf("--dir did not write outside the project: %v", err)
	}
}

// The bound is the project, and a destination can leave it without a symbolic
// link: a custom Agent's ProjectDir is joined to the root and can climb out of
// it. That is refused too, so the message must not blame a link that is not
// there.
func TestProjectDirCannotClimbOutOfTheProject(t *testing.T) {
	climbing := skillembed.Agent{
		Name:       "climbing",
		Title:      "Climbing",
		ProjectDir: "../shared/skills",
	}
	in, _ := newInstaller(t, skillembed.WithAgents(climbing))

	_, err := in.Targets(skillembed.InstallOptions{})
	if !errors.Is(err, skillembed.ErrProjectEscapes) {
		t.Fatalf("Targets = %v, want ErrProjectEscapes", err)
	}
	if strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("the message blames a symbolic link that is not there: %v", err)
	}
}

// reducedInstaller returns an installer carrying only the named skills, the
// way a later version of a tool that dropped one does. The files are copied
// rather than re-authored, so a skill it keeps still hashes to what is already
// on the disk.
func reducedInstaller(t *testing.T, root string, keep ...string) *skillembed.Installer {
	t.Helper()
	kept := map[string]bool{}
	for _, name := range keep {
		kept[name] = true
	}
	files := fstest.MapFS{}
	err := fs.WalkDir(installTestSkills, "testdata/skills", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rest, ok := strings.CutPrefix(p, "testdata/skills/")
		if !ok {
			return nil
		}
		dir, _, _ := strings.Cut(rest, "/")
		if !kept[dir] {
			return nil
		}
		data, err := installTestSkills.ReadFile(p)
		if err != nil {
			return err
		}
		files[p] = &fstest.MapFile{Data: data}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(files, "testdata/skills"),
		skillembed.WithToolName("testtool"),
		skillembed.WithVersion("v2.0.0"),
		skillembed.WithProjectRoot(root),
		skillembed.WithOutput(&bytes.Buffer{}),
	)
}

// installedDir is the one directory a single agent run writes to.
func installedDir(t *testing.T, in *skillembed.Installer, o skillembed.InstallOptions) string {
	t.Helper()
	targets, err := in.Targets(o)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(targets))
	}
	return targets[0].Dir
}

func actionFor(results []skillembed.InstallResult, name string) skillembed.Action {
	for _, r := range results {
		if r.Skill.Name == name {
			return r.Action
		}
	}
	return ""
}

func stateFor(statuses []skillembed.InstallStatus, name string) skillembed.State {
	for _, st := range statuses {
		if st.Skill.Name == name {
			return st.State
		}
	}
	return ""
}

// Every walk starts from the embedded set, so a skill dropped between two
// versions used to be unreachable once installed: install passed it by, list
// did not mention it, and uninstall left it behind for good.
func TestUpgradeRemovesASkillTheBinaryNoLongerCarries(t *testing.T) {
	ctx := t.Context()
	for _, scope := range []skillembed.Scope{skillembed.ScopeProject, skillembed.ScopeUser} {
		t.Run(string(scope), func(t *testing.T) {
			old, root := newInstaller(t)
			if scope == skillembed.ScopeUser {
				t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "home"))
			}
			opts := skillembed.InstallOptions{
				Scope:  scope,
				Agents: []skillembed.AgentSelector{"claude-code"},
			}
			if _, err := old.Install(ctx, opts); err != nil {
				t.Fatal(err)
			}
			dir := installedDir(t, old, opts)
			if _, err := os.Stat(filepath.Join(dir, "bare-skill")); err != nil {
				t.Fatalf("the first install did not land: %v", err)
			}

			next := reducedInstaller(t, root, "demo-skill")

			t.Run("list reports it", func(t *testing.T) {
				statuses, err := next.Status(ctx, opts)
				if err != nil {
					t.Fatal(err)
				}
				if got := stateFor(statuses, "bare-skill"); got != skillembed.StateOrphaned {
					t.Errorf("state = %q, want %s", got, skillembed.StateOrphaned)
				}
			})

			t.Run("a named run does not sweep", func(t *testing.T) {
				named := opts
				named.Names = []string{"demo-skill"}
				if _, err := next.Install(ctx, named); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(filepath.Join(dir, "bare-skill")); err != nil {
					t.Errorf("`install demo-skill` removed a skill it was not asked about: %v", err)
				}
			})

			t.Run("a dry run does not sweep", func(t *testing.T) {
				dry := opts
				dry.DryRun = true
				results, err := next.Install(ctx, dry)
				if err != nil {
					t.Fatal(err)
				}
				if got := actionFor(results, "bare-skill"); got != skillembed.ActionRemoved {
					t.Errorf("dry run action = %q, want %s", got, skillembed.ActionRemoved)
				}
				if _, err := os.Stat(filepath.Join(dir, "bare-skill")); err != nil {
					t.Errorf("the dry run removed it: %v", err)
				}
			})

			t.Run("a full install sweeps", func(t *testing.T) {
				results, err := next.Install(ctx, opts)
				if err != nil {
					t.Fatal(err)
				}
				if got := actionFor(results, "bare-skill"); got != skillembed.ActionRemoved {
					t.Errorf("action = %q, want %s", got, skillembed.ActionRemoved)
				}
				if _, err := os.Stat(filepath.Join(dir, "bare-skill")); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("the dropped skill survived: %v", err)
				}
				if _, err := os.Stat(filepath.Join(dir, "demo-skill")); err != nil {
					t.Errorf("the skill still carried was removed too: %v", err)
				}
			})
		})
	}
}

// uninstall means everything this tool put there, including what it no longer
// carries. Otherwise a dropped skill can never be reached again.
func TestUninstallTakesOrphansWithIt(t *testing.T) {
	ctx := t.Context()
	old, root := newInstaller(t)
	opts := skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"claude-code"}}
	if _, err := old.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}
	dir := installedDir(t, old, opts)

	next := reducedInstaller(t, root, "demo-skill")
	if _, err := next.Uninstall(ctx, opts); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"demo-skill", "bare-skill"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("uninstall left %s behind: %v", name, err)
		}
	}
}

// The sweep claims a directory on this tool's own stamp with a digest that
// still matches, and on nothing weaker. Everything else in the skills
// directory has to survive it, with or without --force.
func TestTheSweepClaimsOnlyThisToolsOwnWork(t *testing.T) {
	ctx := t.Context()
	old, root := newInstaller(t)
	opts := skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"claude-code"}}
	if _, err := old.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}
	dir := installedDir(t, old, opts)

	write := func(name, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, skillembed.SkillFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("handwritten", "---\nname: handwritten\n---\nmine\n")
	if err := os.MkdirAll(filepath.Join(dir, "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A second tool built on this library, writing its own skills into the
	// same directory. Its stamps are as well formed as this one's and its
	// digests check out, so x-embedded-by is the only thing between its work
	// and this sweep. A hand-made fixture with a bogus digest does not test
	// that: the digest would turn it away first, whether or not the name was
	// ever read.
	other := skillembed.NewInstaller(
		skillembed.MustSkillsFromFS(installTestSkills, "testdata/skills"),
		skillembed.WithToolName("othertool"),
		skillembed.WithVersion("v1.0.0"),
		skillembed.WithOutput(&bytes.Buffer{}),
	)
	otherDir := filepath.Join(dir, "from-othertool")
	if _, err := other.Install(ctx, skillembed.InstallOptions{Dir: otherDir}); err != nil {
		t.Fatal(err)
	}
	// Into this tool's own directory as well, under a name it does not carry.
	copyTree(t, filepath.Join(otherDir, "bare-skill"), filepath.Join(dir, "othertools-skill"))
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	next := reducedInstaller(t, root, "demo-skill")
	forced := opts
	forced.Force = true
	if _, err := next.Install(ctx, forced); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"handwritten", "othertools-skill", "not-a-skill", "notes.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("the sweep took %s, which is not this tool's: %v", name, err)
		}
	}
}

// A directory this tool wrote and somebody has since edited is no longer what
// this tool left there, which is exactly what a shipped skill copied and then
// made into one of the user's own looks like. Nothing is being installed over
// it, so there is no conflict for --force to resolve, and --force must not
// reach it: the tool asks for --force whenever anything is modified, so a
// sweep that escalated with it would destroy that work on the tool's own
// advice.
func TestAnEditedForkIsNeverSwept(t *testing.T) {
	ctx := t.Context()
	old, root := newInstaller(t)
	opts := skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"claude-code"}}
	if _, err := old.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}
	dir := installedDir(t, old, opts)

	fork := filepath.Join(dir, "bare-skill-mine")
	copyTree(t, filepath.Join(dir, "bare-skill"), fork)
	body, err := os.ReadFile(filepath.Join(fork, skillembed.SkillFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fork, skillembed.SkillFile), append(body, []byte("\nMy own additions.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	next := reducedInstaller(t, root, "demo-skill")
	for _, run := range []struct {
		name string
		call func(context.Context, skillembed.InstallOptions) ([]skillembed.InstallResult, error)
	}{
		{"install", next.Install},
		{"uninstall", next.Uninstall},
	} {
		for _, force := range []bool{false, true} {
			o := opts
			o.Force = force
			results, _ := run.call(ctx, o)
			for _, r := range results {
				if r.Path == fork {
					t.Errorf("%s (force=%v) reported the fork: %s", run.name, force, r.Action)
				}
			}
			if _, err := os.Stat(filepath.Join(fork, skillembed.SkillFile)); err != nil {
				t.Fatalf("%s (force=%v) took the fork: %v", run.name, force, err)
			}
		}
	}
}

// A file system that folds case answers to more than one spelling, so a skill
// renamed to another spelling of itself reads as an embedded row and as an
// orphan at once, and both rows are the same directory. Removing the orphan
// after writing the skill deleted what the run had just installed, and said
// "updated" and "removed" on its way to exit 0.
func TestASpellingOfAnInstalledSkillIsNotSweptAway(t *testing.T) {
	ctx := t.Context()
	in, root := newInstaller(t)
	if !foldsCase(t, root) {
		t.Skip("this file system does not fold case, so the two names are two directories")
	}
	opts := skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"claude-code"}}
	if _, err := in.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}
	dir := installedDir(t, in, opts)
	if err := os.Rename(filepath.Join(dir, "bare-skill"), filepath.Join(dir, "Bare-Skill")); err != nil {
		t.Fatal(err)
	}

	results, err := in.Install(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Action == skillembed.ActionRemoved {
			t.Errorf("removed %s at %s, which is the skill this run installed", r.Skill.Name, r.Path)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "bare-skill", skillembed.SkillFile)); err != nil {
		t.Errorf("the installed skill is gone: %v", err)
	}
}

// foldsCase reports whether the file system under dir answers to more than one
// spelling of a name.
func foldsCase(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	if err := os.MkdirAll(probe, 0o755); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(probe) }()
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

// When skillfs.Write cannot put the new tree in place it leaves the old one
// beside the destination and names it in the error, for the user to go and
// recover. It is a verbatim copy of an installation, stamp and digest and all,
// so the sweep would otherwise claim it and take away the only copy.
func TestTheSweepLeavesARescuedInstallationAlone(t *testing.T) {
	ctx := t.Context()
	old, root := newInstaller(t)
	opts := skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"claude-code"}}
	if _, err := old.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}
	dir := installedDir(t, old, opts)
	rescued := filepath.Join(dir, ".bare-skill.old-1234")
	if err := os.Rename(filepath.Join(dir, "bare-skill"), rescued); err != nil {
		t.Fatal(err)
	}

	next := reducedInstaller(t, root, "demo-skill")
	results, err := next.Install(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Path == rescued {
			t.Errorf("the sweep reported the rescue copy: %s", r.Action)
		}
	}
	if _, err := os.Stat(filepath.Join(rescued, skillembed.SkillFile)); err != nil {
		t.Errorf("the rescue copy the error told the user to recover is gone: %v", err)
	}
}

// list prints an orphan's name, so uninstall has to accept it. Resolving names
// against the embedded set alone told the user that a skill the tool had just
// listed does not exist.
func TestAnOrphanCanBeNamed(t *testing.T) {
	ctx := t.Context()
	old, root := newInstaller(t)
	opts := skillembed.InstallOptions{Agents: []skillembed.AgentSelector{"claude-code"}}
	if _, err := old.Install(ctx, opts); err != nil {
		t.Fatal(err)
	}
	dir := installedDir(t, old, opts)
	next := reducedInstaller(t, root, "demo-skill")

	named := opts
	named.Names = []string{"bare-skill"}
	results, err := next.Uninstall(ctx, named)
	if err != nil {
		t.Fatalf("Uninstall = %v, want nil", err)
	}
	if got := actionFor(results, "bare-skill"); got != skillembed.ActionRemoved {
		t.Errorf("action = %q, want %s", got, skillembed.ActionRemoved)
	}
	if _, err := os.Stat(filepath.Join(dir, "bare-skill")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the named orphan survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "demo-skill")); err != nil {
		t.Errorf("a skill that was not named was removed: %v", err)
	}

	unknown := opts
	unknown.Names = []string{"never-existed"}
	if _, err := next.Status(ctx, unknown); !errors.Is(err, skillembed.ErrUnknownSkill) {
		t.Errorf("Status = %v, want ErrUnknownSkill", err)
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rest, _ := filepath.Rel(from, p)
		target := filepath.Join(to, rest)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}
