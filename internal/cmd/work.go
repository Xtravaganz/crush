package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/crush/internal/workflow"
	"github.com/spf13/cobra"
)

var workCmd = &cobra.Command{
	Use:   "work [title]",
	Short: "Create or activate the local workflow for the current Git branch",
	Args:  cobra.ArbitraryArgs,
	Example: `
# Activate the branch-scoped local workflow
crush work

# Optionally give the local task a human-readable title
crush work "Refactor user update flow"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := ResolveCwd(cmd)
		if err != nil {
			return err
		}
		result, err := workflow.ActivateLocalContext(cmd.Context(), cwd, strings.Join(args, " "))
		if err != nil {
			return err
		}

		action := "Activated"
		if result.Created {
			action = "Created"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s local workflow for branch %s -> %s\n", action, result.Branch, relativeOrBase(cwd, result.ContextPath))
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Active workflow context -> %s\n", relativeOrBase(cwd, result.ActivePath))
		return nil
	},
}
