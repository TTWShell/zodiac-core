package cli

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TTWShell/zodiac-core/internal/scaffold"
)

func newProjectCommand() *cobra.Command {
	var modulePath, output, coreVersion, corePath string
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Generate a standalone Go service with Cobra and Fx",
		Example: "  zodiac new demo-service --module example.com/demo-service\n" +
			"  zodiac new demo-service --module example.com/demo-service --core-path . --output /tmp/demo-service",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := scaffold.Generate(scaffold.Options{
				Name: args[0], Module: modulePath, Output: output,
				CoreVersion: coreVersion, CorePath: corePath,
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"Created project at %s\n\nNext steps:\n  cd %s\n  go mod tidy\n  APPLICATION_ENVIRONMENT=develop go run ./cmd/%s serve --config-dir ./config\n",
				dir, shellQuote(dir), args[0])
			return err
		},
	}
	cmd.Flags().StringVar(&modulePath, "module", "", "Go module path for the generated project (required)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Destination directory (default: ./<name>; must be empty or absent)")
	cmd.Flags().StringVar(&coreVersion, "core-version", builtCoreVersion(), "Published ZodiacCore version (defaults to this CLI's release version)")
	cmd.Flags().StringVar(&corePath, "core-path", "", "Local ZodiacCore checkout; writes an explicit development replace directive")
	// The flag is defined immediately above, so marking it required cannot fail.
	if err := cmd.MarkFlagRequired("module"); err != nil {
		panic(err)
	}
	return cmd
}

func builtCoreVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Path == scaffold.CoreModule && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return ""
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
