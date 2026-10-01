package commands

import "testing"

func TestIsControlCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/reload", true},
		{"  /reload  ", true},
		{"!reload", true},
		{"/RELOAD", true},
		{"/reload@picoclaw_bot", true},
		{"/switch model to other", true},
		{"/help", false},
		{"/clear", false},
		{"/stop", false},
		{"reload", false},
		{"please /reload", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsControlCommand(tt.input); got != tt.want {
			t.Errorf("IsControlCommand(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// Every control command must exist as a builtin, so a rename upstream
// cannot silently leave the gate pointing at nothing.
func TestControlCommandsAreBuiltins(t *testing.T) {
	builtins := map[string]bool{}
	for _, def := range BuiltinDefinitions() {
		builtins[def.Name] = true
	}
	for name := range controlCommands {
		if !builtins[name] {
			t.Errorf("control command %q is not a builtin definition", name)
		}
	}
}
