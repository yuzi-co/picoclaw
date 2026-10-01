//go:build !ironkvm

package main

import (
	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/pkg/updater"
)

const updateCommandEnabled = true

// addUpdateCommand registers the self-updater. The ironkvm build leaves it
// out (update_ironkvm.go): IronKVM installs and updates PicoClaw itself, and
// the updater only checks a SHA-256 published in the same release, unsigned.
func addUpdateCommand(cmd *cobra.Command) {
	cmd.AddCommand(updater.NewUpdateCommand("picoclaw"))
}
