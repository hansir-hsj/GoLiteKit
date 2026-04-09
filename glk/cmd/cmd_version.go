package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the current release of glk.
const Version = "v0.1.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of glk",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("glk %s\n", Version)
	},
}
