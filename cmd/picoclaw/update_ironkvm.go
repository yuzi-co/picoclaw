//go:build ironkvm

package main

import "github.com/spf13/cobra"

const updateCommandEnabled = false

// addUpdateCommand is a no-op in the ironkvm build, which has no self-updater.
func addUpdateCommand(*cobra.Command) {}
