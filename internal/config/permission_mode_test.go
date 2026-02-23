package config

import "testing"

func TestCycleMode(t *testing.T) {
	tests := []struct {
		current         PermissionMode
		bypassAvailable bool
		want            PermissionMode
	}{
		// Standard cycle without bypass.
		{ModeDefault, false, ModeAcceptEdits},
		{ModeAcceptEdits, false, ModePlan},
		{ModePlan, false, ModeDefault},

		// Standard cycle with bypass available.
		{ModeDefault, true, ModeAcceptEdits},
		{ModeAcceptEdits, true, ModePlan},
		{ModePlan, true, ModeBypassPermissions},
		{ModeBypassPermissions, true, ModeDefault},

		// DontAsk always goes to default.
		{ModeDontAsk, false, ModeDefault},
		{ModeDontAsk, true, ModeDefault},

		// BypassPermissions without availability still goes to default.
		{ModeBypassPermissions, false, ModeDefault},
	}

	for _, tt := range tests {
		t.Run(string(tt.current)+"_bypass="+boolStr(tt.bypassAvailable), func(t *testing.T) {
			got := CycleMode(tt.current, tt.bypassAvailable)
			if got != tt.want {
				t.Errorf("CycleMode(%q, %v) = %q, want %q", tt.current, tt.bypassAvailable, got, tt.want)
			}
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestValidPermissionMode(t *testing.T) {
	valid := []string{"default", "plan", "acceptEdits", "bypassPermissions", "dontAsk"}
	for _, s := range valid {
		if !ValidPermissionMode(s) {
			t.Errorf("ValidPermissionMode(%q) = false, want true", s)
		}
	}

	invalid := []string{"", "invalid", "Plan", "DEFAULT", "bypass"}
	for _, s := range invalid {
		if ValidPermissionMode(s) {
			t.Errorf("ValidPermissionMode(%q) = true, want false", s)
		}
	}
}

func TestModeInfoMap(t *testing.T) {
	// All modes should have info entries.
	modes := []PermissionMode{ModeDefault, ModePlan, ModeAcceptEdits, ModeBypassPermissions, ModeDontAsk}
	for _, mode := range modes {
		info, ok := ModeInfoMap[mode]
		if !ok {
			t.Errorf("ModeInfoMap missing entry for %q", mode)
			continue
		}
		if info.Mode != mode {
			t.Errorf("ModeInfoMap[%q].Mode = %q", mode, info.Mode)
		}
		if info.Title == "" {
			t.Errorf("ModeInfoMap[%q].Title is empty", mode)
		}
		if info.ShortTitle == "" {
			t.Errorf("ModeInfoMap[%q].ShortTitle is empty", mode)
		}
	}
}

func TestCycleModeFullLoop(t *testing.T) {
	// Verify cycling through all modes without bypass returns to default.
	mode := ModeDefault
	mode = CycleMode(mode, false) // default → acceptEdits
	if mode != ModeAcceptEdits {
		t.Fatalf("step 1: got %q", mode)
	}
	mode = CycleMode(mode, false) // acceptEdits → plan
	if mode != ModePlan {
		t.Fatalf("step 2: got %q", mode)
	}
	mode = CycleMode(mode, false) // plan → default (no bypass)
	if mode != ModeDefault {
		t.Fatalf("step 3: got %q", mode)
	}
}

func TestCycleModeFullLoopWithBypass(t *testing.T) {
	// Verify cycling through all modes with bypass returns to default.
	mode := ModeDefault
	mode = CycleMode(mode, true) // default → acceptEdits
	if mode != ModeAcceptEdits {
		t.Fatalf("step 1: got %q", mode)
	}
	mode = CycleMode(mode, true) // acceptEdits → plan
	if mode != ModePlan {
		t.Fatalf("step 2: got %q", mode)
	}
	mode = CycleMode(mode, true) // plan → bypassPermissions
	if mode != ModeBypassPermissions {
		t.Fatalf("step 3: got %q", mode)
	}
	mode = CycleMode(mode, true) // bypassPermissions → default
	if mode != ModeDefault {
		t.Fatalf("step 4: got %q", mode)
	}
}

func TestToolPermissionContextBypassAvailable(t *testing.T) {
	ctx := NewToolPermissionContext()

	if ctx.GetIsBypassAvailable() {
		t.Error("bypass should be unavailable by default")
	}

	ctx.SetIsBypassAvailable(true)
	if !ctx.GetIsBypassAvailable() {
		t.Error("bypass should be available after SetIsBypassAvailable(true)")
	}

	ctx.SetIsBypassAvailable(false)
	if ctx.GetIsBypassAvailable() {
		t.Error("bypass should be unavailable after SetIsBypassAvailable(false)")
	}
}
