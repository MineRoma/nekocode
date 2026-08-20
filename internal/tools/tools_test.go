package tools

import (
	"testing"

	"github.com/m1neroma/neko/internal/core"
)

func TestDefinitionsByMode(t *testing.T) {
	registry := &Registry{}
	build := registry.Definitions(core.ModeBuild)
	plan := registry.Definitions(core.ModePlan)
	reverse := registry.Definitions(core.ModeReverse)

	if len(build) == 0 {
		t.Fatal("build mode must expose tools")
	}
	if len(plan) >= len(build) {
		t.Fatalf("plan mode must be restricted: %d plan vs %d build", len(plan), len(build))
	}
	// Reverse mode inspects and rewrites artifacts, so it needs the full surface.
	if len(reverse) != len(build) {
		t.Fatalf("reverse mode must expose every tool: %d vs %d", len(reverse), len(build))
	}
	for _, tool := range plan {
		if !tool.ReadOnly {
			t.Fatalf("plan mode exposed a mutating tool: %s", tool.Name)
		}
	}
	// An unrecognized mode must not silently drop to read-only.
	if len(registry.Definitions("nonsense")) != len(build) {
		t.Fatal("unknown modes should behave like build")
	}
}

func TestDefinitionsIncludeWriteToolsOutsidePlan(t *testing.T) {
	registry := &Registry{}
	for _, mode := range []string{core.ModeBuild, core.ModeReverse} {
		names := map[string]bool{}
		for _, tool := range registry.Definitions(mode) {
			names[tool.Name] = true
		}
		for _, required := range []string{"write_file", "replace_in_file", "run_command", "read_file", "search"} {
			if !names[required] {
				t.Fatalf("%s mode is missing %s", mode, required)
			}
		}
	}
}
