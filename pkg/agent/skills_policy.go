//go:build !ironkvm

package agent

import "github.com/sipeed/picoclaw/pkg/config"

// applyBuildSkillsPolicy lets a build flavour restrict skill loading. The
// stock build always loads skills; see skills_policy_ironkvm.go.
func applyBuildSkillsPolicy(*ContextBuilder, *config.Config) {}
