package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/workflow"
	"github.com/stretchr/testify/require"
)

func TestWorkflowCheckpointTextRemovesCodeAndTruncates(t *testing.T) {
	text := "Finding before\n```ts\nconst secret = 'do-not-store';\n```\nFinding after"
	got := workflowCheckpointText(text, 80)
	require.Contains(t, got, "Finding before")
	require.Contains(t, got, "[code omitted]")
	require.Contains(t, got, "Finding after")
	require.NotContains(t, got, "do-not-store")

	got = workflowCheckpointText(strings.Repeat("a", 20), 8)
	require.Equal(t, "aaaaaaaa …", got)
}

func TestPersistWorkflowCheckpointWritesOnlyMappedWorker(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "work-main.yaml"), []byte(`version: 1
shared: {}
workers:
  code:
    marker: KEEP_CODE
  review: {}
`), 0o644))
	require.NoError(t, workflow.SetActiveContext(root, "work-main.yaml"))
	require.NoError(t, workflow.SetWorkerSession(root, "code", "session-code"))
	require.NoError(t, workflow.SetWorkerSession(root, "review", "session-review"))

	a := &sessionAgent{workingDir: root}
	require.NoError(t, a.persistWorkflowCheckpoint(
		"session-review",
		"Review the change",
		"Found a regression risk.\n```ts\nconst ignored = true\n```",
		"assistant_final",
	))

	data, err := os.ReadFile(filepath.Join(contextDir, "work-main.yaml"))
	require.NoError(t, err)
	text := string(data)
	require.Contains(t, text, "KEEP_CODE")
	require.Contains(t, text, "checkpoint:")
	require.Contains(t, text, "source: assistant_final")
	require.Contains(t, text, "Review the change")
	require.Contains(t, text, "Found a regression risk")
	require.Contains(t, text, "[code omitted]")
	require.NotContains(t, text, "const ignored")
}

func TestPersistWorkflowCheckpointReplacesPreviousCheckpoint(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "work-main.yaml"), []byte("version: 1\nworkers:\n  code: {}\n"), 0o644))
	require.NoError(t, workflow.SetActiveContext(root, "work-main.yaml"))
	require.NoError(t, workflow.SetWorkerSession(root, "code", "session-code"))

	a := &sessionAgent{workingDir: root}
	require.NoError(t, a.persistWorkflowCheckpoint(
		"session-code",
		"OLD_REQUEST_SHOULD_DISAPPEAR",
		"First final response",
		"assistant_final",
	))
	require.NoError(t, a.persistWorkflowCheckpoint(
		"session-code",
		"",
		"Compacted durable summary",
		"compaction",
	))

	data, err := os.ReadFile(filepath.Join(contextDir, "work-main.yaml"))
	require.NoError(t, err)
	text := string(data)
	require.Contains(t, text, "source: compaction")
	require.Contains(t, text, "Compacted durable summary")
	require.NotContains(t, text, "OLD_REQUEST_SHOULD_DISAPPEAR")
}

func TestPersistWorkflowCheckpointIgnoresNonWorkflowSession(t *testing.T) {
	a := &sessionAgent{workingDir: t.TempDir()}
	require.NoError(t, a.persistWorkflowCheckpoint("ordinary-session", "request", "result", "assistant_final"))
}
