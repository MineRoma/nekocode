package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m1neroma/neko/internal/config"
	"github.com/m1neroma/neko/internal/core"
)

func writeSkill(t *testing.T, root, name, body string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestParseFrontmatter(t *testing.T) {
	descriptor := parseFrontmatter("---\nname: binary-triage\nmode: reverse\nsummary: First pass on an unknown binary.\n---\n\n# Body\n")
	if descriptor.Name != "binary-triage" || descriptor.Mode != "reverse" {
		t.Fatalf("unexpected descriptor %#v", descriptor)
	}
	if descriptor.Summary != "First pass on an unknown binary." {
		t.Fatalf("unexpected summary %q", descriptor.Summary)
	}
}

func TestParseFrontmatterWithoutHeader(t *testing.T) {
	descriptor := parseFrontmatter("# Just a heading\n\nSome text.\n")
	if descriptor.Name != "" || descriptor.Mode != "" || descriptor.Summary != "" {
		t.Fatalf("expected an empty descriptor, got %#v", descriptor)
	}
}

func TestParseFrontmatterAcceptsDescriptionAndQuotes(t *testing.T) {
	descriptor := parseFrontmatter("---\nname: \"quoted-name\"\ndescription: 'single quoted'\n---\n")
	if descriptor.Name != "quoted-name" {
		t.Fatalf("unexpected name %q", descriptor.Name)
	}
	if descriptor.Summary != "single quoted" {
		t.Fatalf("unexpected summary %q", descriptor.Summary)
	}
}

func TestDiscoverFindsSkillDirectories(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "binary-triage", "---\nname: binary-triage\nmode: reverse\nsummary: Triage.\n---\n")
	writeSkill(t, root, "ghidra-workflow", "---\nname: ghidra-workflow\nmode: reverse\nsummary: Decompile.\n---\n")
	// A directory with no SKILL.md is ignored rather than failing the scan.
	if err := os.MkdirAll(filepath.Join(root, "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A loose file at the top level is ignored too.
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# index"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("expected 2 skills, got %#v", found)
	}
	if found[0].Name != "binary-triage" || found[1].Name != "ghidra-workflow" {
		t.Fatalf("expected alphabetical order, got %#v", found)
	}
	if found[0].Mode != "reverse" {
		t.Fatalf("unexpected mode %q", found[0].Mode)
	}
}

func TestDiscoverFallsBackToDirectoryName(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "unnamed-skill", "# No frontmatter here\n")
	found, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Name != "unnamed-skill" {
		t.Fatalf("unexpected result %#v", found)
	}
}

func TestDiscoverRejectsEmptyDirectory(t *testing.T) {
	if _, err := Discover(t.TempDir()); err == nil {
		t.Fatal("expected an error when no skills are present")
	}
}

func TestAddRecordsModeAndSummary(t *testing.T) {
	root := t.TempDir()
	dir := writeSkill(t, root, "binary-triage", "---\nname: binary-triage\nmode: reverse\nsummary: Triage an unknown binary.\n---\n")
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store, err := config.Open()
	if err != nil {
		t.Fatal(err)
	}
	skillStore := New(store)
	if err := skillStore.Add("binary-triage", dir); err != nil {
		t.Fatal(err)
	}
	items := skillStore.List()
	if len(items) != 1 {
		t.Fatalf("expected one skill, got %#v", items)
	}
	if items[0].Mode != "reverse" || items[0].Summary != "Triage an unknown binary." {
		t.Fatalf("frontmatter was not recorded: %#v", items[0])
	}
}

func TestForModeFiltersByMode(t *testing.T) {
	root := t.TempDir()
	reverseDir := writeSkill(t, root, "binary-triage", "---\nmode: reverse\nsummary: Triage.\n---\n")
	planDir := writeSkill(t, root, "spec-writing", "---\nmode: plan\nsummary: Specs.\n---\n")
	anyDir := writeSkill(t, root, "house-style", "---\nsummary: Style rules.\n---\n")
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store, err := config.Open()
	if err != nil {
		t.Fatal(err)
	}
	skillStore := New(store)
	for name, dir := range map[string]string{"binary-triage": reverseDir, "spec-writing": planDir, "house-style": anyDir} {
		if err := skillStore.Add(name, dir); err != nil {
			t.Fatal(err)
		}
	}

	reverse := skillStore.ForMode(core.ModeReverse)
	if len(reverse) != 2 {
		t.Fatalf("expected the reverse skill plus the untagged one, got %#v", reverse)
	}
	build := skillStore.ForMode(core.ModeBuild)
	if len(build) != 1 || build[0].Name != "house-style" {
		t.Fatalf("expected only the untagged skill in build mode, got %#v", build)
	}
	plan := skillStore.ForMode(core.ModePlan)
	if len(plan) != 2 {
		t.Fatalf("expected the plan skill plus the untagged one, got %#v", plan)
	}
}

func TestAddRequiresSkillFile(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store, err := config.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := New(store).Add("empty", empty); err == nil {
		t.Fatal("expected a directory without SKILL.md to be rejected")
	}
}

func TestBundledForModeShipsReverseSkills(t *testing.T) {
	items := BundledForMode(core.ModeReverse)
	if len(items) < 20 {
		t.Fatalf("expected the bundled reverse set, got %d skills", len(items))
	}
	seen := map[string]bool{}
	for _, item := range items {
		if item.Name == "" || item.Summary == "" {
			t.Fatalf("bundled skill is missing frontmatter: %#v", item)
		}
		if item.Mode != core.ModeReverse {
			t.Fatalf("%s should be tagged reverse, got %q", item.Name, item.Mode)
		}
		if !strings.Contains(item.Body, "# ") {
			t.Fatalf("%s has no body content", item.Name)
		}
		if seen[item.Name] {
			t.Fatalf("duplicate bundled skill %q", item.Name)
		}
		seen[item.Name] = true
	}
	for _, required := range []string{"binary-triage", "ghidra-workflow", "anti-reversing", "notes-and-reporting"} {
		if !seen[required] {
			t.Fatalf("bundled set is missing %s", required)
		}
	}
	// Build and Plan ship nothing, so switching to them must not register skills.
	if got := BundledForMode(core.ModeBuild); len(got) != 0 {
		t.Fatalf("build mode should ship no skills, got %d", len(got))
	}
}

func TestInstallBundledRegistersOnceAndIsIdempotent(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store, err := config.Open()
	if err != nil {
		t.Fatal(err)
	}
	skillStore := New(store)
	root := t.TempDir()

	added, err := skillStore.InstallBundled(root, core.ModeReverse)
	if err != nil {
		t.Fatal(err)
	}
	expected := len(BundledForMode(core.ModeReverse))
	if len(added) != expected {
		t.Fatalf("expected %d skills registered, got %d", expected, len(added))
	}
	// Files must exist on disk and be loadable by name.
	body, err := skillStore.Load("binary-triage")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Binary triage") {
		t.Fatalf("unexpected body %q", body[:minInt(80, len(body))])
	}

	// A second call registers nothing new.
	again, err := skillStore.InstallBundled(root, core.ModeReverse)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("expected no new registrations, got %#v", again)
	}
	if total := len(skillStore.List()); total != expected {
		t.Fatalf("expected %d skills total, got %d", expected, total)
	}
}

func TestInstallBundledPreservesUserState(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store, err := config.Open()
	if err != nil {
		t.Fatal(err)
	}
	skillStore := New(store)
	root := t.TempDir()
	if _, err := skillStore.InstallBundled(root, core.ModeReverse); err != nil {
		t.Fatal(err)
	}
	// Disabling a skill must survive a reinstall.
	if err := store.Update(func(cfg *config.Config) error {
		for i := range cfg.Skills {
			if cfg.Skills[i].Name == "ctf-reversing" {
				cfg.Skills[i].Enabled = false
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := skillStore.InstallBundled(root, core.ModeReverse); err != nil {
		t.Fatal(err)
	}
	for _, skill := range skillStore.List() {
		if skill.Name == "ctf-reversing" && skill.Enabled {
			t.Fatal("reinstall must not re-enable a skill the user disabled")
		}
	}
	for _, skill := range skillStore.ForMode(core.ModeReverse) {
		if skill.Name == "ctf-reversing" {
			t.Fatal("a disabled skill must not be offered to the model")
		}
	}
}

func TestLoadFallsBackToEmbeddedBody(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store, err := config.Open()
	if err != nil {
		t.Fatal(err)
	}
	skillStore := New(store)
	root := t.TempDir()
	if _, err := skillStore.InstallBundled(root, core.ModeReverse); err != nil {
		t.Fatal(err)
	}
	// Deleting the unpacked file must not break the skill.
	if err := os.RemoveAll(filepath.Join(root, core.ModeReverse, "binary-triage")); err != nil {
		t.Fatal(err)
	}
	body, err := skillStore.Load("binary-triage")
	if err != nil {
		t.Fatalf("expected the embedded copy to be used: %v", err)
	}
	if !strings.Contains(body, "Binary triage") {
		t.Fatal("embedded fallback returned the wrong content")
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
