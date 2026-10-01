//go:build ironkvm

package agent

import (
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/skills"
)

// applyBuildSkillsPolicy stops the ironkvm build from loading skills unless
// tools.skills.enabled is set (it is off by default in this build). A
// SKILL.md dropped into the workspace, the global skills directory or the
// builtin directory would otherwise be summarised into every system prompt
// and could be pulled in with /use (upstream #3075).
func applyBuildSkillsPolicy(cb *ContextBuilder, cfg *config.Config) {
	if cb == nil || (cfg != nil && cfg.Tools.IsToolEnabled("skills")) {
		return
	}
	// A loader with no directories lists and loads nothing.
	cb.skillsLoader = &skills.SkillsLoader{}
}
