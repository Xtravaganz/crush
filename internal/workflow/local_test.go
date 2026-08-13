package workflow

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestEnsureLocalContextCreatesBranchScopedWorkflow(t *testing.T) {
	root := initTestGitRepo(t, "feature/user-email")
	require.NoError(t, runGit(root, "remote", "add", "origin", "git@gitlab.example.com:team/project.git"))

	result, created, err := EnsureLocalContext(context.Background(), root)
	require.NoError(t, err)
	require.True(t, created)
	require.True(t, result.Created)
	require.Equal(t, "feature/user-email", result.Branch)
	require.Equal(t, "work-feature-user-email.yaml", filepath.Base(result.ContextPath))

	state, ok, err := LoadActiveState(root)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "work-feature-user-email.yaml", state.ActiveContext)

	doc := readTestYAML(t, result.ContextPath)
	require.Equal(t, "local", nestedString(doc, "source", "type"))
	require.Equal(t, "feature/user-email", nestedString(doc, "source", "branch"))
	require.Equal(t, "team/project", nestedString(doc, "shared", "repo", "project"))
	require.Equal(t, "feature/user-email", nestedString(doc, "shared", "repo", "branch"))
	require.NotNil(t, doc["workers"])

	excludePath := filepath.Join(root, ".git", "info", "exclude")
	exclude, err := os.ReadFile(excludePath)
	require.NoError(t, err)
	require.Contains(t, string(exclude), ".crush/context/")
}

func TestEnsureLocalContextDoesNotReplaceActiveIssue(t *testing.T) {
	root := initTestGitRepo(t, "feature/issue-32")
	contextDir := filepath.Join(root, ".crush", "context")
	require.NoError(t, os.MkdirAll(contextDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "issue-32.yaml"), []byte("shared: {}\nworkers: {}\n"), 0o644))
	require.NoError(t, SetActiveContext(root, "issue-32.yaml"))

	_, created, err := EnsureLocalContext(context.Background(), root)
	require.NoError(t, err)
	require.False(t, created)

	state, ok, err := LoadActiveState(root)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "issue-32.yaml", state.ActiveContext)
}

func TestActivateLocalContextPreservesWorkersAndCanSwitchBranches(t *testing.T) {
	root := initTestGitRepo(t, "feature/cache")
	first, err := ActivateLocalContext(context.Background(), root, "Cache refactor")
	require.NoError(t, err)

	doc := readTestYAML(t, first.ContextPath)
	doc["workers"] = map[string]any{
		"code": map[string]any{"marker": "KEEP_ME"},
	}
	data, err := yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(first.ContextPath, data, 0o644))
	require.NoError(t, SetWorkerSession(root, "code", "session-cache"))

	refreshed, err := ActivateLocalContext(context.Background(), root, "Cache refactor")
	require.NoError(t, err)
	require.False(t, refreshed.Created)
	doc = readTestYAML(t, refreshed.ContextPath)
	require.Equal(t, "KEEP_ME", nestedString(doc, "workers", "code", "marker"))
	state, _, err := LoadActiveState(root)
	require.NoError(t, err)
	require.Equal(t, "session-cache", state.WorkerSessions["code"])

	require.NoError(t, runGit(root, "checkout", "-b", "feature/docker"))
	second, err := ActivateLocalContext(context.Background(), root, "Docker cleanup")
	require.NoError(t, err)
	require.True(t, second.Created)
	require.Equal(t, "work-feature-docker.yaml", filepath.Base(second.ContextPath))
	state, _, err = LoadActiveState(root)
	require.NoError(t, err)
	require.Equal(t, "work-feature-docker.yaml", state.ActiveContext)
	require.Empty(t, state.WorkerSessions)
}

func TestEnsureLocalContextFollowsBranchForLocalWorkflow(t *testing.T) {
	root := initTestGitRepo(t, "feature/one")
	first, created, err := EnsureLocalContext(context.Background(), root)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, "work-feature-one.yaml", filepath.Base(first.ContextPath))
	require.NoError(t, SetWorkerSession(root, "code", "session-one"))

	require.NoError(t, runGit(root, "checkout", "-b", "feature/two"))
	second, switched, err := EnsureLocalContext(context.Background(), root)
	require.NoError(t, err)
	require.True(t, switched)
	require.Equal(t, "work-feature-two.yaml", filepath.Base(second.ContextPath))

	state, ok, err := LoadActiveState(root)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "work-feature-two.yaml", state.ActiveContext)
	require.Empty(t, state.WorkerSessions)
}

func TestEnsureLocalContextOutsideGitIsNoop(t *testing.T) {
	_, created, err := EnsureLocalContext(context.Background(), t.TempDir())
	require.NoError(t, err)
	require.False(t, created)
}

func initTestGitRepo(t *testing.T, branch string) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, runGit(root, "init"))
	require.NoError(t, runGit(root, "config", "user.email", "test@example.com"))
	require.NoError(t, runGit(root, "config", "user.name", "Test User"))
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("test\n"), 0o644))
	require.NoError(t, runGit(root, "add", "README.md"))
	require.NoError(t, runGit(root, "commit", "--no-gpg-sign", "-m", "init"))
	require.NoError(t, runGit(root, "checkout", "-b", branch))
	return root
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run()
}

func readTestYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(data, &doc))
	return doc
}

func nestedString(root map[string]any, keys ...string) string {
	var current any = root
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[key]
	}
	value, _ := current.(string)
	return value
}
