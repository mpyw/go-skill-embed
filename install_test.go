package skillembed_test

import (
	"bytes"
	"embed"
	"errors"
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

	targets, err := in.Targets(skillembed.InstallOptions{Agents: []string{"all"}, Scope: "project"})
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
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")
	opts := skillembed.InstallOptions{Dir: dest, Names: []string{"demo-skill"}}

	results, err := in.Install(opts)
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
	statuses, err := in.Status(opts)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != skillembed.StateUpToDate {
		t.Fatalf("state after install = %s, want %s", statuses[0].State, skillembed.StateUpToDate)
	}
	results, err = in.Install(opts)
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
	statuses, err = in.Status(opts)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != skillembed.StateModified {
		t.Fatalf("state after an edit = %s, want %s", statuses[0].State, skillembed.StateModified)
	}
	if _, err := in.Install(opts); err == nil {
		t.Error("install overwrote an edited skill without --force")
	}

	forced := opts
	forced.Force = true
	results, err = in.Install(forced)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Action != skillembed.ActionUpdated {
		t.Errorf("forced install = %s, want %s", results[0].Action, skillembed.ActionUpdated)
	}

	removed, err := in.Uninstall(opts)
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
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")
	opts := skillembed.InstallOptions{Dir: dest, Names: []string{"demo-skill"}}

	if _, err := in.Install(opts); err != nil {
		t.Fatal(err)
	}

	// A second tool of a different name finds the same directory occupied.
	other, _ := newInstaller(t, skillembed.WithToolName("othertool"), skillembed.WithProjectRoot(root))
	statuses, err := other.Status(opts)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != skillembed.StateForeign {
		t.Fatalf("state = %s, want %s", statuses[0].State, skillembed.StateForeign)
	}
	if _, err := other.Install(opts); err == nil {
		t.Error("install replaced another tool's skill without --force")
	}

	results, err := other.Uninstall(opts)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Action != skillembed.ActionSkipped {
		t.Errorf("uninstall = %+v, want it skipped", results[0])
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")

	results, err := in.Install(skillembed.InstallOptions{Dir: dest, DryRun: true})
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
	out := &bytes.Buffer{}
	in, root := newInstaller(t, skillembed.WithOutput(out))
	dest := filepath.Join(root, "skills")

	if err := in.Run([]string{"install", "--dir", dest, "demo-skill"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "installed") {
		t.Errorf("install said:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dest, "demo-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := in.Run([]string{"list", "--dir", dest}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "up-to-date") {
		t.Errorf("list said:\n%s", out)
	}

	if err := in.Run([]string{"nonsense"}); err == nil {
		t.Error("an unknown subcommand was accepted")
	}
	if err := in.Run([]string{"install", "--agent", "nonsense"}); err == nil {
		t.Error("an unknown agent was accepted")
	}
}

// A SKILL.md with no frontmatter gains one on install. It must still read as
// up-to-date afterwards, which it does not if the added block cannot be
// stripped back off for the digest.
func TestSkillWithoutFrontmatterInstallsClean(t *testing.T) {
	in, root := newInstaller(t)
	opts := skillembed.InstallOptions{Dir: filepath.Join(root, "skills"), Names: []string{"bare-skill"}}

	if _, err := in.Install(opts); err != nil {
		t.Fatal(err)
	}
	statuses, err := in.Status(opts)
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
	targets, err = in.Targets(skillembed.InstallOptions{Agents: []string{"all"}, Scope: "project"})
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
	in, root := newInstaller(t)
	opts := skillembed.InstallOptions{Dir: filepath.Join(root, "skills"), Names: []string{"demo-skill"}}

	if _, err := in.Install(opts); err != nil {
		t.Fatal(err)
	}
	junk := filepath.Join(root, "skills", "demo-skill", ".DS_Store")
	if err := os.WriteFile(junk, []byte("\x00\x01binary"), 0o644); err != nil {
		t.Fatal(err)
	}

	statuses, err := in.Status(opts)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].State != skillembed.StateUpToDate {
		t.Errorf("state = %s, want %s", statuses[0].State, skillembed.StateUpToDate)
	}
	results, err := in.Install(opts)
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
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")

	// Something else owns demo-skill's name.
	if err := os.MkdirAll(filepath.Join(dest, "demo-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "demo-skill", "SKILL.md"), []byte("hand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := in.Install(skillembed.InstallOptions{Dir: dest})
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
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")
	opts := skillembed.InstallOptions{Dir: dest}

	if _, err := in.Install(opts); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dest, "demo-skill", "link.txt")
	if err := os.Symlink(filepath.Join(root, "elsewhere"), link); err != nil {
		t.Fatal(err)
	}

	statuses, err := in.Status(opts)
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
	results, err := in.Install(forced)
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

	if _, err := in.Uninstall(forced); err != nil {
		t.Fatalf("forced uninstall failed: %v", err)
	}
}

// A front end wants to map "you typed a bad value" to a different exit code
// than "the disk is full", and so does a test.
func TestErrorsAreMatchable(t *testing.T) {
	in, root := newInstaller(t)
	dest := filepath.Join(root, "skills")

	for _, c := range []struct {
		name string
		opts skillembed.InstallOptions
		want error
	}{
		{"unknown agent", skillembed.InstallOptions{Agents: []string{"nonsense"}}, skillembed.ErrUnknownAgent},
		{"unknown scope", skillembed.InstallOptions{Scope: "nonsense"}, skillembed.ErrUnknownScope},
		{"unknown skill", skillembed.InstallOptions{Dir: dest, Names: []string{"nonsense"}}, skillembed.ErrUnknownSkill},
		{"nothing selected", skillembed.InstallOptions{Agents: []string{""}}, skillembed.ErrNoAgentSelected},
		// Dir wins over agent and scope, but a bad value beside it is still a
		// mistake, and saying nothing about it was the old behaviour.
		{"dir with a bad scope", skillembed.InstallOptions{Dir: dest, Scope: "nonsense"}, skillembed.ErrUnknownScope},
		{"dir with a bad agent", skillembed.InstallOptions{Dir: dest, Agents: []string{"nonsense"}}, skillembed.ErrUnknownAgent},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := in.Status(c.opts)
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want it to wrap %v", err, c.want)
			}
		})
	}
}
