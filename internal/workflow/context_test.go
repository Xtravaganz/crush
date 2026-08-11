package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/skills"
	"github.com/stretchr/testify/require"
)

func TestResolveActiveDefaultsToSharedAndOwnWorker(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "issue-32.yaml"), []byte(`version: 1
shared:
  issue:
    iid: 32
    title: Test issue
workers:
  code:
    marker: CODE_ONLY
  tests:
    marker: TESTS_ONLY
  review:
    marker: REVIEW_ONLY
`), 0o644))
	require.NoError(t, SetActiveContext(root, "issue-32.yaml"))

	resolved, ok, err := ResolveActive(root, "review", skills.ContextPolicy{})
	require.NoError(t, err)
	require.True(t, ok)
	text := string(resolved.Content)
	require.Contains(t, text, "REVIEW_ONLY")
	require.Contains(t, text, "Test issue")
	require.NotContains(t, text, "CODE_ONLY")
	require.NotContains(t, text, "TESTS_ONLY")
	require.Contains(t, text, "workers.review")
}

func TestResolveActiveIncludeAddsOnlyRequestedPath(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "issue-32.yaml"), []byte(`version: 1
shared:
  issue:
    iid: 32
workers:
  code:
    changes:
      - file: foo.go
    notes: DO_NOT_INCLUDE
  review:
    findings: []
`), 0o644))
	require.NoError(t, SetActiveContext(root, "issue-32.yaml"))

	resolved, ok, err := ResolveActive(root, "review", skills.ContextPolicy{
		Include: []string{"workers.code.changes"},
	})
	require.NoError(t, err)
	require.True(t, ok)
	text := string(resolved.Content)
	require.Contains(t, text, "foo.go")
	require.NotContains(t, text, "DO_NOT_INCLUDE")
}

func TestResolveActiveFallsBackToSingleIssueFile(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "issue-7.yaml"), []byte(`shared: {}
workers: {}
`), 0o644))

	resolved, ok, err := ResolveActive(root, "tests", skills.ContextPolicy{})
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, strings.HasSuffix(resolved.RelativePath, "issue-7.yaml"))
}
