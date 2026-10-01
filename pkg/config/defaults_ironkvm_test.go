//go:build ironkvm

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func assertHardenedDefaults(t *testing.T, cfg *Config) {
	t.Helper()
	checks := []struct {
		name string
		got  bool
		want bool
	}{
		{"agents.defaults.restrict_to_workspace", cfg.Agents.Defaults.RestrictToWorkspace, true},
		{"agents.defaults.allow_read_outside_workspace", cfg.Agents.Defaults.AllowReadOutsideWorkspace, false},
		{"tools.exec.enabled", cfg.Tools.Exec.Enabled, false},
		{"tools.exec.allow_remote", cfg.Tools.Exec.AllowRemote, false},
		{"tools.exec.enable_deny_patterns", cfg.Tools.Exec.EnableDenyPatterns, true},
		{"tools.web.enabled", cfg.Tools.Web.Enabled, false},
		{"tools.web_fetch.enabled", cfg.Tools.WebFetch.Enabled, false},
		{"tools.skills.enabled", cfg.Tools.Skills.Enabled, false},
		{"tools.find_skills.enabled", cfg.Tools.FindSkills.Enabled, false},
		{"tools.install_skill.enabled", cfg.Tools.InstallSkill.Enabled, false},
		{"tools.spawn.enabled", cfg.Tools.Spawn.Enabled, false},
		{"tools.spawn_status.enabled", cfg.Tools.SpawnStatus.Enabled, false},
		{"tools.subagent.enabled", cfg.Tools.Subagent.Enabled, false},
		{"tools.cron.enabled", cfg.Tools.Cron.Enabled, false},
		{"tools.cron.allow_command", cfg.Tools.Cron.AllowCommand, false},
		{"heartbeat.enabled", cfg.Heartbeat.Enabled, false},
		// The KVM control path must stay available.
		{"tools.read_file.enabled", cfg.Tools.ReadFile.Enabled, true},
		{"tools.load_image.enabled", cfg.Tools.LoadImage.Enabled, true},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	for _, registry := range cfg.Tools.Skills.Registries {
		if registry != nil && registry.Enabled {
			t.Errorf("skill registry %q enabled, want disabled", registry.Name)
		}
	}
	for _, tool := range []string{"exec", "web", "web_fetch", "skills", "find_skills", "install_skill", "spawn", "subagent", "cron"} {
		if cfg.Tools.IsToolEnabled(tool) {
			t.Errorf("IsToolEnabled(%q) = true, want false", tool)
		}
	}
}

func TestDefaultConfig_IronKVMHardened(t *testing.T) {
	assertHardenedDefaults(t, DefaultConfig())
}

// A config.json that does not mention these settings, like one written
// before this build, must still get the hardened values.
func TestLoadConfig_IronKVMHardenedWhenKeysMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"version": 3, "agents": {"defaults": {"workspace": "` + filepath.ToSlash(dir) + `"}}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	assertHardenedDefaults(t, cfg)
}

// Turning exec back on in config.json does not re-enable remote execution
// or drop the deny patterns.
func TestLoadConfig_IronKVMExecEnabledKeepsGuards(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"version": 3, "tools": {"exec": {"enabled": true}}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Tools.Exec.Enabled {
		t.Fatal("tools.exec.enabled = false, want the explicit true")
	}
	if cfg.Tools.Exec.AllowRemote {
		t.Error("tools.exec.allow_remote = true, want false")
	}
	if !cfg.Tools.Exec.EnableDenyPatterns {
		t.Error("tools.exec.enable_deny_patterns = false, want true")
	}
}
