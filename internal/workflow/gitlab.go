package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type GitLabIssue struct {
	IID         int              `json:"iid"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	State       string           `json:"state"`
	WebURL      string           `json:"web_url"`
	UpdatedAt   string           `json:"updated_at"`
	Labels      []string         `json:"labels"`
	Milestone   *GitLabMilestone `json:"milestone"`
	Assignees   []GitLabUser     `json:"assignees"`
	Author      GitLabUser       `json:"author"`
}

type GitLabMilestone struct {
	Title string `json:"title"`
}

type GitLabUser struct {
	Name     string `json:"name"`
	Username string `json:"username"`
}

type GitRemote struct {
	URL     string
	Scheme  string
	Host    string
	Project string
}

type ImportResult struct {
	IssuePath  string
	ActivePath string
	Refreshed  bool
	Remote     GitRemote
	Issue      GitLabIssue
}

// ImportGitLabIssue imports or refreshes a GitLab issue into the local workflow
// context. Existing worker state and non-GitLab shared fields are preserved.
func ImportGitLabIssue(ctx context.Context, projectRoot string, iid int) (ImportResult, error) {
	if iid <= 0 {
		return ImportResult{}, fmt.Errorf("issue IID must be greater than zero")
	}

	remoteURL, err := gitOutput(ctx, projectRoot, "remote", "get-url", "origin")
	if err != nil {
		return ImportResult{}, fmt.Errorf("resolve git origin: %w", err)
	}
	remote, err := ParseGitRemote(remoteURL)
	if err != nil {
		return ImportResult{}, err
	}

	issue, err := fetchGitLabIssue(ctx, projectRoot, remote, iid)
	if err != nil {
		return ImportResult{}, err
	}
	if issue.IID == 0 {
		issue.IID = iid
	}

	branch, _ := gitOutput(ctx, projectRoot, "branch", "--show-current")
	contextDir := filepath.Join(projectRoot, filepath.FromSlash(contextDirName))
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		return ImportResult{}, fmt.Errorf("create workflow context directory: %w", err)
	}

	filename := fmt.Sprintf("issue-%d.yaml", iid)
	issuePath := filepath.Join(contextDir, filename)
	doc := map[string]any{}
	refreshed := false
	if data, readErr := os.ReadFile(issuePath); readErr == nil {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return ImportResult{}, fmt.Errorf("parse existing workflow context %s: %w", issuePath, err)
		}
		refreshed = true
	} else if !os.IsNotExist(readErr) {
		return ImportResult{}, readErr
	}
	if doc == nil {
		doc = map[string]any{}
	}

	mergeImportedIssue(doc, remote, branch, issue)
	data, err := yaml.Marshal(doc)
	if err != nil {
		return ImportResult{}, fmt.Errorf("marshal workflow context: %w", err)
	}
	if err := writeFileAtomic(issuePath, data, 0o644); err != nil {
		return ImportResult{}, fmt.Errorf("write workflow context: %w", err)
	}
	if err := SetActiveContext(projectRoot, filename); err != nil {
		return ImportResult{}, err
	}

	return ImportResult{
		IssuePath:  issuePath,
		ActivePath: filepath.Join(contextDir, activeFileName),
		Refreshed:  refreshed,
		Remote:     remote,
		Issue:      issue,
	}, nil
}

func mergeImportedIssue(doc map[string]any, remote GitRemote, branch string, issue GitLabIssue) {
	doc["version"] = 1
	doc["source"] = map[string]any{
		"type":       "gitlab",
		"host":       remote.Host,
		"project":    remote.Project,
		"iid":        issue.IID,
		"web_url":    issue.WebURL,
		"updated_at": issue.UpdatedAt,
	}

	shared, ok := doc["shared"].(map[string]any)
	if !ok || shared == nil {
		shared = map[string]any{}
		doc["shared"] = shared
	}
	shared["repo"] = map[string]any{
		"host":    remote.Host,
		"project": remote.Project,
		"remote":  remote.URL,
		"branch":  branch,
	}

	issueMap := map[string]any{
		"iid":         issue.IID,
		"title":       issue.Title,
		"description": issue.Description,
		"state":       issue.State,
		"web_url":     issue.WebURL,
		"updated_at":  issue.UpdatedAt,
		"labels":      issue.Labels,
	}
	if issue.Milestone != nil {
		issueMap["milestone"] = issue.Milestone.Title
	} else {
		issueMap["milestone"] = nil
	}
	if len(issue.Assignees) > 0 {
		assignees := make([]map[string]string, 0, len(issue.Assignees))
		for _, assignee := range issue.Assignees {
			assignees = append(assignees, map[string]string{
				"name":     assignee.Name,
				"username": assignee.Username,
			})
		}
		issueMap["assignees"] = assignees
	} else {
		issueMap["assignees"] = []any{}
	}
	if issue.Author.Username != "" || issue.Author.Name != "" {
		issueMap["author"] = map[string]string{
			"name":     issue.Author.Name,
			"username": issue.Author.Username,
		}
	}
	shared["issue"] = issueMap

	if _, ok := doc["workers"].(map[string]any); !ok {
		doc["workers"] = map[string]any{}
	}
}

func fetchGitLabIssue(ctx context.Context, projectRoot string, remote GitRemote, iid int) (GitLabIssue, error) {
	if token := firstNonEmpty(os.Getenv("GITLAB_TOKEN"), os.Getenv("GLAB_TOKEN")); token != "" {
		return fetchGitLabIssueAPI(ctx, remote, iid, token)
	}
	if _, err := exec.LookPath("glab"); err == nil {
		return fetchGitLabIssueGLab(ctx, projectRoot, iid)
	}
	return GitLabIssue{}, fmt.Errorf("GitLab authentication unavailable: set GITLAB_TOKEN (or GLAB_TOKEN), or install and authenticate glab")
}

func fetchGitLabIssueAPI(ctx context.Context, remote GitRemote, iid int, token string) (GitLabIssue, error) {
	scheme := remote.Scheme
	if scheme != "http" && scheme != "https" {
		scheme = "https"
	}
	base := scheme + "://" + remote.Host
	projectID := url.PathEscape(remote.Project)
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/issues/%d", base, projectID, iid)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return GitLabIssue{}, err
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return GitLabIssue{}, fmt.Errorf("GitLab issue request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return GitLabIssue{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return GitLabIssue{}, fmt.Errorf("GitLab API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var issue GitLabIssue
	if err := json.Unmarshal(body, &issue); err != nil {
		return GitLabIssue{}, fmt.Errorf("decode GitLab issue: %w", err)
	}
	return issue, nil
}

func fetchGitLabIssueGLab(ctx context.Context, projectRoot string, iid int) (GitLabIssue, error) {
	cmd := exec.CommandContext(ctx, "glab", "issue", "view", strconv.Itoa(iid), "--output", "json")
	cmd.Dir = projectRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return GitLabIssue{}, fmt.Errorf("glab issue view failed: %s", msg)
	}
	var issue GitLabIssue
	if err := json.Unmarshal(stdout.Bytes(), &issue); err != nil {
		return GitLabIssue{}, fmt.Errorf("decode glab issue JSON: %w", err)
	}
	return issue, nil
}

// ParseGitRemote supports the common HTTPS/HTTP/SSH URL forms plus the SCP-like
// git@host:group/project.git syntax used by GitLab.
func ParseGitRemote(remoteURL string) (GitRemote, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return GitRemote{}, fmt.Errorf("empty git remote URL")
	}
	remote := GitRemote{URL: remoteURL}

	if strings.Contains(remoteURL, "://") {
		u, err := url.Parse(remoteURL)
		if err != nil {
			return GitRemote{}, fmt.Errorf("parse git remote %q: %w", remoteURL, err)
		}
		remote.Scheme = u.Scheme
		remote.Host = u.Host
		remote.Project = strings.TrimPrefix(u.Path, "/")
	} else {
		// SCP-like syntax: [user@]host:namespace/project.git
		colon := strings.Index(remoteURL, ":")
		if colon <= 0 || colon == len(remoteURL)-1 {
			return GitRemote{}, fmt.Errorf("unsupported git remote URL %q", remoteURL)
		}
		hostPart := remoteURL[:colon]
		if at := strings.LastIndex(hostPart, "@"); at >= 0 {
			hostPart = hostPart[at+1:]
		}
		remote.Scheme = "ssh"
		remote.Host = hostPart
		remote.Project = remoteURL[colon+1:]
	}

	remote.Project = strings.TrimSuffix(strings.Trim(remote.Project, "/"), ".git")
	if remote.Host == "" || remote.Project == "" {
		return GitRemote{}, fmt.Errorf("could not derive GitLab host/project from git remote %q", remoteURL)
	}
	return remote, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
