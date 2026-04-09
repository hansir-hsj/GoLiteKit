package cmd

import (
	"fmt"

	glk "github.com/hansir-hsj/GoLiteKit"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of glk",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("glk %s\n", glk.Version)
	},
}
