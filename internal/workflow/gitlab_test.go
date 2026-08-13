package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGitRemote(t *testing.T) {
	tests := []struct {
		name    string
		remote  string
		host    string
		project string
	}{
		{"scp", "git@gitlab.example.com:group/sub/project.git", "gitlab.example.com", "group/sub/project"},
		{"https", "https://gitlab.example.com/group/project.git", "gitlab.example.com", "group/project"},
		{"ssh", "ssh://git@gitlab.example.com/group/project.git", "gitlab.example.com", "group/project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote, err := ParseGitRemote(tt.remote)
			require.NoError(t, err)
			require.Equal(t, tt.host, remote.Host)
			require.Equal(t, tt.project, remote.Project)
		})
	}
}

func TestMergeImportedIssuePreservesWorkersAndLocalSharedFields(t *testing.T) {
	doc := map[string]any{
		"shared": map[string]any{
			"decisions": []any{"keep me"},
		},
		"workers": map[string]any{
			"code": map[string]any{"marker": "KEEP_WORKER"},
		},
	}
	remote := GitRemote{URL: "git@gitlab.example.com:group/project.git", Host: "gitlab.example.com", Project: "group/project"}
	issue := GitLabIssue{IID: 32, Title: "Updated title", Description: "Updated description", State: "opened", Labels: []string{"bug"}}

	mergeImportedIssue(doc, remote, "feature/test", issue)

	shared := doc["shared"].(map[string]any)
	require.Equal(t, []any{"keep me"}, shared["decisions"])
	workers := doc["workers"].(map[string]any)
	require.Equal(t, "KEEP_WORKER", workers["code"].(map[string]any)["marker"])
	require.Equal(t, "Updated title", shared["issue"].(map[string]any)["title"])
}
