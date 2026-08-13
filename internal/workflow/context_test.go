package workflow

import (
	"os"
	"path/filepath"
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
    verdict: pending
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

func TestResolveActiveRequiresExplicitActivePointer(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "issue-7.yaml"), []byte(`shared: {}
workers: {}
`), 0o644))

	_, ok, err := ResolveActive(root, "tests", skills.ContextPolicy{})
	require.NoError(t, err)
	require.False(t, ok)
}

func TestWorkerSessionStateIsScopedToActiveContext(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "issue-32.yaml"), []byte("shared: {}\nworkers: {}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "issue-33.yaml"), []byte("shared: {}\nworkers: {}\n"), 0o644))

	require.NoError(t, SetActiveContext(root, "issue-32.yaml"))
	require.NoError(t, SetWorkerSession(root, "code", "session-code-32"))
	require.NoError(t, SetWorkerSession(root, "tests", "session-tests-32"))

	state, ok, err := LoadActiveState(root)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "tests", state.ActiveSkill)
	require.Equal(t, "session-code-32", state.WorkerSessions["code"])
	require.Equal(t, "session-tests-32", state.WorkerSessions["tests"])

	// Refreshing the same work item preserves its worker sessions.
	require.NoError(t, SetActiveContext(root, "issue-32.yaml"))
	state, _, err = LoadActiveState(root)
	require.NoError(t, err)
	require.Equal(t, "session-code-32", state.WorkerSessions["code"])

	// Activating a different work item starts with clean worker/session state.
	require.NoError(t, SetActiveContext(root, "issue-33.yaml"))
	state, _, err = LoadActiveState(root)
	require.NoError(t, err)
	require.Empty(t, state.ActiveSkill)
	require.Empty(t, state.WorkerSessions)
}

func TestResolvedContextCarriesMemoryDisciplineWithoutSourceCode(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "issue-32.yaml"), []byte(`shared:
  issue:
    iid: 32
workers:
  code:
    working_set:
      - file: src/UserService.ts
        symbol: updateUser
`), 0o644))
	require.NoError(t, SetActiveContext(root, "issue-32.yaml"))

	resolved, ok, err := ResolveActive(root, "code", skills.ContextPolicy{})
	require.NoError(t, err)
	require.True(t, ok)
	text := string(resolved.Content)
	require.Contains(t, text, "src/UserService.ts")
	require.Contains(t, text, "updateUser")
	require.Contains(t, text, "workflow_update")
	require.Contains(t, text, "never full source or raw tool output")
	require.NotContains(t, text, "memory_rules")
}

func TestUpdateWorkflowMemoryScopesWorkerDataAndGlobalFindings(t *testing.T) {
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
      - file: src/foo.ts
  review:
    verdict: pending
findings:
  review-001:
    source: review
    status: open
    summary: Existing finding
`), 0o644))
	require.NoError(t, SetActiveContext(root, "issue-32.yaml"))
	require.NoError(t, SetWorkerSession(root, "code", "session-code"))
	require.NoError(t, SetWorkerSession(root, "review", "session-review"))

	worker, source, err := UpdateWorkflowMemory(root, "session-review", map[string]any{
		"findings": map[string]any{
			"review-002": map[string]any{"severity": "high", "summary": "New finding"},
		},
		"risks":   []any{"regression"},
		"verdict": "changes_requested",
	})
	require.NoError(t, err)
	require.Equal(t, "review", worker)
	require.Equal(t, ".crush/context/issue-32.yaml", source)

	doc, err := readYAMLMap(filepath.Join(contextDir, "issue-32.yaml"))
	require.NoError(t, err)
	code, ok := getPath(doc, "workers.code.changes")
	require.True(t, ok)
	require.Contains(t, code, map[string]any{"file": "src/foo.ts"})

	findings, ok := getPath(doc, "findings")
	require.True(t, ok)
	require.Equal(t, map[string]any{
		"review-001": map[string]any{"source": "review", "status": "open", "summary": "Existing finding"},
		"review-002": map[string]any{"source": "review", "status": "open", "severity": "high", "summary": "New finding"},
	}, findings)
	_, workerFindings := getPath(doc, "workers.review.findings")
	require.False(t, workerFindings)
	verdict, ok := getPath(doc, "workers.review.verdict")
	require.True(t, ok)
	require.Equal(t, "changes_requested", verdict)
	risks, ok := getPath(doc, "workers.review.risks")
	require.True(t, ok)
	require.Equal(t, []any{"regression"}, risks)
}

func TestUpdateWorkflowMemoryRejectsUnmappedSession(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "work-main.yaml"), []byte("version: 1\nworkers: {}\n"), 0o644))
	require.NoError(t, SetActiveContext(root, "work-main.yaml"))
	require.NoError(t, SetWorkerSession(root, "review", "session-review"))

	_, _, err := UpdateWorkflowMemory(root, "session-code", map[string]any{"verdict": "approved"})
	require.ErrorContains(t, err, "not mapped to an active workflow worker")

	doc, readErr := readYAMLMap(filepath.Join(contextDir, "work-main.yaml"))
	require.NoError(t, readErr)
	_, exists := getPath(doc, "workers.review.verdict")
	require.False(t, exists)
}

func TestWorkflowFindingLifecycleAcrossWorkers(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "work-main.yaml"), []byte("version: 1\nworkers: {}\n"), 0o644))
	require.NoError(t, SetActiveContext(root, "work-main.yaml"))
	require.NoError(t, SetWorkerSession(root, "review", "session-review"))
	require.NoError(t, SetWorkerSession(root, "code", "session-code"))

	_, _, err := UpdateWorkflowMemory(root, "session-review", map[string]any{
		"findings": map[string]any{
			"review-001": map[string]any{
				"summary": "Possible precision loss in yield calculation",
				"owner":   "code",
			},
		},
	})
	require.NoError(t, err)

	_, _, err = UpdateWorkflowMemory(root, "session-code", map[string]any{
		"findings": map[string]any{
			"review-001": map[string]any{
				"status": "resolved",
				"note":   "Calculation uses decimal arithmetic now.",
			},
		},
	})
	require.NoError(t, err)

	doc, err := readYAMLMap(filepath.Join(contextDir, "work-main.yaml"))
	require.NoError(t, err)
	findings, ok := getPath(doc, "findings")
	require.True(t, ok)
	require.Equal(t, map[string]any{
		"review-001": map[string]any{
			"source":  "review",
			"status":  "resolved",
			"summary": "Possible precision loss in yield calculation",
			"owner":   "code",
			"note":    "Calculation uses decimal arithmetic now.",
		},
	}, findings)
}

func TestWorkflowFindingDefaultsStatusAndSource(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "work-main.yaml"), []byte("version: 1\nworkers: {}\n"), 0o644))
	require.NoError(t, SetActiveContext(root, "work-main.yaml"))
	require.NoError(t, SetWorkerSession(root, "review", "session-review"))

	_, _, err := UpdateWorkflowMemory(root, "session-review", map[string]any{
		"findings": map[string]any{
			"review-001": map[string]any{"summary": "Possible precision loss"},
		},
	})
	require.NoError(t, err)

	doc, err := readYAMLMap(filepath.Join(contextDir, "work-main.yaml"))
	require.NoError(t, err)
	finding, ok := getPath(doc, "findings.review-001")
	require.True(t, ok)
	require.Equal(t, map[string]any{
		"summary": "Possible precision loss",
		"status":  "open",
		"source":  "review",
	}, finding)
}

func TestResolveActiveIncludesOnlyOpenGlobalFindings(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "work-main.yaml"), []byte(`version: 1
workers:
  code:
    checkpoint:
      summary: Fixed the precision issue.
findings:
  review-001:
    source: review
    status: open
    summary: OPEN_FINDING
  tests-001:
    source: tests
    status: resolved
    summary: RESOLVED_FINDING
  review-002:
    source: review
    status: superseded
    summary: SUPERSEDED_FINDING
`), 0o644))
	require.NoError(t, SetActiveContext(root, "work-main.yaml"))

	resolved, ok, err := ResolveActive(root, "code", skills.ContextPolicy{})
	require.NoError(t, err)
	require.True(t, ok)
	text := string(resolved.Content)
	require.Contains(t, text, "OPEN_FINDING")
	require.Contains(t, text, "Fixed the precision issue")
	require.Contains(t, text, "findings")
	require.Contains(t, text, "resolved")
	require.Contains(t, text, "superseded")
	require.NotContains(t, text, "RESOLVED_FINDING")
	require.NotContains(t, text, "SUPERSEDED_FINDING")
}

func TestWorkerForSession(t *testing.T) {
	root := t.TempDir()
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "work-main.yaml"), []byte("version: 1\nworkers: {}\n"), 0o644))
	require.NoError(t, SetActiveContext(root, "work-main.yaml"))
	require.NoError(t, SetWorkerSession(root, "review", "session-review"))

	worker, ok, err := WorkerForSession(root, "session-review")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "review", worker)

	_, ok, err = WorkerForSession(root, "unknown")
	require.NoError(t, err)
	require.False(t, ok)
}
