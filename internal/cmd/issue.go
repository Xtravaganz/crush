package cmd

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/charmbracelet/crush/internal/workflow"
	"github.com/spf13/cobra"
)

var issueCmd = &cobra.Command{
	Use:   "issue <iid>",
	Short: "Import or refresh a GitLab issue as the active workflow context",
	Args:  cobra.ExactArgs(1),
	Example: `
# Import GitLab issue #32 and make it active
crush issue 32

# Refresh it later; existing worker context is preserved
crush issue 32`,
	RunE: func(cmd *cobra.Command, args []string) error {
		iid, err := strconv.Atoi(args[0])
		if err != nil || iid <= 0 {
			return fmt.Errorf("invalid issue IID %q", args[0])
		}

		cwd, err := ResolveCwd(cmd)
		if err != nil {
			return err
		}
		result, err := workflow.ImportGitLabIssue(cmd.Context(), cwd, iid)
		if err != nil {
			return err
		}

		action := "Imported"
		if result.Refreshed {
			action = "Refreshed"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s GitLab issue #%d -> %s\n", action, iid, relativeOrBase(cwd, result.IssuePath))
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Active workflow context -> %s\n", relativeOrBase(cwd, result.ActivePath))
		return nil
	},
}

func relativeOrBase(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}
