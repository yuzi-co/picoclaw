//go:build !ironkvm

package config

// applyBuildDefaults adjusts DefaultConfig for the build flavour. The stock
// build keeps the upstream defaults; see defaults_ironkvm.go.
func applyBuildDefaults(*Config) {}
