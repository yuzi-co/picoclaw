//go:build ironkvm

package config

// applyBuildDefaults hardens DefaultConfig for IronKVM, where the agent runs
// as root on the KVM and reads text from the controlled host's screen. Only
// defaults change: a value set explicitly in config.json still wins, so
// whoever writes the config (the IronKVM server) must set the same values to
// enforce them on existing installs.
//
// The agent keeps file tools confined to the workspace, load_image and MCP
// (the KVM server's screenshot and input tools); it gets no shell, no
// network fetch or search, no skills, no subagents and no scheduled work.
func applyBuildDefaults(cfg *Config) {
	cfg.Agents.Defaults.RestrictToWorkspace = true
	cfg.Agents.Defaults.AllowReadOutsideWorkspace = false

	// Exec is off. If a config turns it back on, it may not run for
	// remote channels (pico is one) and the deny patterns stay active.
	cfg.Tools.Exec.Enabled = false
	cfg.Tools.Exec.AllowRemote = false
	cfg.Tools.Exec.EnableDenyPatterns = true

	// No web search or fetch (SSRF reports upstream #3074, #3077, #3078).
	cfg.Tools.Web.Enabled = false
	cfg.Tools.Web.PreferNative = false
	cfg.Tools.WebFetch.Enabled = false

	// No skills: nothing to discover or install, and with skills disabled
	// the agent does not load SKILL.md files from the workspace, global or
	// builtin skill directories into the prompt (upstream #3075).
	cfg.Tools.Skills.Enabled = false
	for _, registry := range cfg.Tools.Skills.Registries {
		if registry != nil {
			registry.Enabled = false
		}
	}
	cfg.Tools.FindSkills.Enabled = false
	cfg.Tools.InstallSkill.Enabled = false

	// No spawn or subagents (cross-session leak described in upstream PR #3403).
	cfg.Tools.Spawn.Enabled = false
	cfg.Tools.SpawnStatus.Enabled = false
	cfg.Tools.Subagent.Enabled = false

	// No scheduled work.
	cfg.Tools.Cron.Enabled = false
	cfg.Tools.Cron.AllowCommand = false
	cfg.Heartbeat.Enabled = false
}
