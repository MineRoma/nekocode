package core

import "testing"

func TestNormalizeMode(t *testing.T) {
	cases := map[string]string{
		"build":    ModeBuild,
		"Build":    ModeBuild,
		"plan":     ModePlan,
		"PLAN":     ModePlan,
		" plan ":   ModePlan,
		"reverse":  ModeReverse,
		"Reverse":  ModeReverse,
		"":         ModeBuild,
		"nonsense": ModeBuild,
	}
	for input, want := range cases {
		if got := NormalizeMode(input); got != want {
			t.Fatalf("NormalizeMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNextModeCyclesThroughThreeModes(t *testing.T) {
	if got := NextMode(ModeBuild); got != ModePlan {
		t.Fatalf("build should advance to plan, got %q", got)
	}
	if got := NextMode(ModePlan); got != ModeReverse {
		t.Fatalf("plan should advance to reverse, got %q", got)
	}
	if got := NextMode(ModeReverse); got != ModeBuild {
		t.Fatalf("reverse should wrap to build, got %q", got)
	}
	// An unknown stored mode must still produce a usable next mode.
	if got := NextMode("garbage"); got != ModePlan {
		t.Fatalf("unknown mode should normalize to build then advance, got %q", got)
	}
	// Three presses return to the start.
	mode := ModeBuild
	for i := 0; i < 3; i++ {
		mode = NextMode(mode)
	}
	if mode != ModeBuild {
		t.Fatalf("three cycles should return to build, got %q", mode)
	}
}

func TestModeLabel(t *testing.T) {
	cases := map[string]string{
		ModeBuild:   "Build",
		ModePlan:    "Plan",
		ModeReverse: "Reverse",
		"":          "Build",
		"REVERSE":   "Reverse",
	}
	for input, want := range cases {
		if got := ModeLabel(input); got != want {
			t.Fatalf("ModeLabel(%q) = %q, want %q", input, got, want)
		}
	}
}
