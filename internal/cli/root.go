// Package cli implements the Zodiac developer commands.
package cli

import "github.com/spf13/cobra"

// Execute runs the Zodiac developer CLI.
func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "zodiac",
		Short:         "Create Go services with ZodiacCore conventions",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newProjectCommand())
	return cmd
}
