//go:build ironkvm

package providers

// sdkProvidersOmitted is true in builds that stub the SDK-backed providers
// (Azure, Claude/Codex OAuth, GitHub Copilot).
const sdkProvidersOmitted = true
