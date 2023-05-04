package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:   "template2helm",
		Short: "Template2helm converts an OpenShift Template into a Helm Chart.",
		Long: `This is a customization of the Template2helm project which converts an OpenShift Template into a Helm Chart.
      For more info, check out https://github.com/dedalus-enterprise-architect/template2helm`,
	}
)

// Execute - entrypoint for CLI tool
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
