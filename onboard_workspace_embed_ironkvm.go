//go:build ironkvm

package picoclaw

import "embed"

// OnboardWorkspace embeds the onboarding workspace template without the
// default skills: the ironkvm build does not load skills, so onboarding does
// not copy them either.
//
//go:embed workspace/AGENT.md workspace/SOUL.md workspace/USER.md workspace/memory
var OnboardWorkspace embed.FS
