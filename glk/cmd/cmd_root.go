package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "glk",
	Short: "glk is a CLI tool for GoLiteKit",
	Long:  "glk is a CLI tool for GoLiteKit",
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	// Execute prints the error itself; usage is only useful for argument errors.
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%sError: %s%s\n", "\x1b[31m", err, "\x1b[0m")
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(versionCmd)
}
