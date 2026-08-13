package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LocalResult describes a local, branch-scoped workflow context.
type LocalResult struct {
	ContextPath string
	ActivePath  string
	Created     bool
	Branch      string
	Remote      string
}

// EnsureLocalContext creates and activates a branch-scoped local workflow only
// when the project has no active workflow yet. Outside a Git repository it is a
// no-op so Crush keeps its normal behavior.
func EnsureLocalContext(ctx context.Context, projectRoot string) (LocalResult, bool, error) {
	state, ok, err := LoadActiveState(projectRoot)
	if err != nil {
		return LocalResult{}, false, err
	}
	if ok && state.ActiveContext != "" {
		// Explicit issue/external workflows always win. Local workflows follow
		// the current Git branch automatically so switching branches does not
		// accidentally keep another branch's working memory active.
		sourceType, sourceBranch, err := activeContextSource(projectRoot, state.ActiveContext)
		if err != nil {
			return LocalResult{}, false, err
		}
		if sourceType != "local" {
			return LocalResult{}, false, nil
		}
		branch, branchErr := currentGitBranch(ctx, projectRoot)
		if branchErr != nil {
			if errors.Is(branchErr, ErrNotGitRepository) {
				return LocalResult{}, false, nil
			}
			return LocalResult{}, false, branchErr
		}
		if branch == sourceBranch {
			return LocalResult{}, false, nil
		}
	}

	result, err := ActivateLocalContext(ctx, projectRoot, "")
	if err != nil {
		if errors.Is(err, ErrNotGitRepository) {
			return LocalResult{}, false, nil
		}
		return LocalResult{}, false, err
	}
	return result, true, nil
}

var ErrNotGitRepository = errors.New("not a git repository")

// ActivateLocalContext creates or refreshes a branch-scoped local workflow and
// makes it active. title is optional; when provided it is stored as shared.task.
// Existing worker state is preserved when the same branch workflow is reused.
func ActivateLocalContext(ctx context.Context, projectRoot, title string) (LocalResult, error) {
	root, err := gitOutput(ctx, projectRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return LocalResult{}, fmt.Errorf("%w: %v", ErrNotGitRepository, err)
	}
	root = filepath.Clean(root)

	branch, err := currentGitBranch(ctx, root)
	if err != nil {
		return LocalResult{}, err
	}

	remoteURL, _ := gitOutput(ctx, root, "remote", "get-url", "origin")
	if err := ensureWorkflowContextExcluded(ctx, root); err != nil {
		return LocalResult{}, err
	}
	filename := "work-" + slugifyWorkflowName(branch) + ".yaml"
	contextDir := filepath.Join(root, filepath.FromSlash(contextDirName))
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		return LocalResult{}, fmt.Errorf("create workflow context directory: %w", err)
	}

	contextPath := filepath.Join(contextDir, filename)
	doc := map[string]any{}
	created := true
	if data, readErr := os.ReadFile(contextPath); readErr == nil {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return LocalResult{}, fmt.Errorf("parse local workflow context %s: %w", contextPath, err)
		}
		created = false
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return LocalResult{}, readErr
	}
	if doc == nil {
		doc = map[string]any{}
	}

	mergeLocalContext(doc, root, branch, remoteURL, title)
	data, err := yaml.Marshal(doc)
	if err != nil {
		return LocalResult{}, fmt.Errorf("marshal local workflow context: %w", err)
	}
	if err := writeFileAtomic(contextPath, data, 0o644); err != nil {
		return LocalResult{}, fmt.Errorf("write local workflow context: %w", err)
	}
	if err := SetActiveContext(root, filename); err != nil {
		return LocalResult{}, err
	}

	return LocalResult{
		ContextPath: contextPath,
		ActivePath:  filepath.Join(contextDir, activeFileName),
		Created:     created,
		Branch:      branch,
		Remote:      remoteURL,
	}, nil
}

func currentGitBranch(ctx context.Context, projectRoot string) (string, error) {
	if _, err := gitOutput(ctx, projectRoot, "rev-parse", "--show-toplevel"); err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotGitRepository, err)
	}
	branch, _ := gitOutput(ctx, projectRoot, "branch", "--show-current")
	if strings.TrimSpace(branch) != "" {
		return strings.TrimSpace(branch), nil
	}
	shortSHA, err := gitOutput(ctx, projectRoot, "rev-parse", "--short", "HEAD")
	if err != nil || strings.TrimSpace(shortSHA) == "" {
		return "detached", nil
	}
	return "detached-" + strings.TrimSpace(shortSHA), nil
}

func activeContextSource(projectRoot, filename string) (sourceType, branch string, err error) {
	if filename == "" || filepath.Base(filename) != filename || filename == activeFileName {
		return "", "", fmt.Errorf("invalid active workflow context %q", filename)
	}
	path := filepath.Join(projectRoot, filepath.FromSlash(contextDirName), filename)
	doc, err := readYAMLMap(path)
	if err != nil {
		return "", "", fmt.Errorf("read active workflow context %s: %w", path, err)
	}
	source, _ := doc["source"].(map[string]any)
	if source == nil {
		return "", "", nil
	}
	sourceType, _ = source["type"].(string)
	branch, _ = source["branch"].(string)
	return strings.TrimSpace(sourceType), strings.TrimSpace(branch), nil
}

func mergeLocalContext(doc map[string]any, projectRoot, branch, remoteURL, title string) {
	doc["version"] = 1

	source, ok := doc["source"].(map[string]any)
	if !ok || source == nil {
		source = map[string]any{}
		doc["source"] = source
	}
	source["type"] = "local"
	source["branch"] = branch
	if _, exists := source["created_at"]; !exists {
		source["created_at"] = time.Now().UTC().Format(time.RFC3339)
	}

	shared, ok := doc["shared"].(map[string]any)
	if !ok || shared == nil {
		shared = map[string]any{}
		doc["shared"] = shared
	}

	repo := map[string]any{
		"name":   filepath.Base(projectRoot),
		"branch": branch,
	}
	if strings.TrimSpace(remoteURL) != "" {
		repo["remote"] = remoteURL
		if remote, err := ParseGitRemote(remoteURL); err == nil {
			repo["host"] = remote.Host
			repo["project"] = remote.Project
		}
	}
	shared["repo"] = repo

	if strings.TrimSpace(title) != "" {
		task, ok := shared["task"].(map[string]any)
		if !ok || task == nil {
			task = map[string]any{}
		}
		task["title"] = strings.TrimSpace(title)
		shared["task"] = task
	}

	if _, ok := doc["workers"].(map[string]any); !ok {
		doc["workers"] = map[string]any{}
	}
}

func ensureWorkflowContextExcluded(ctx context.Context, projectRoot string) error {
	excludePath, err := gitOutput(ctx, projectRoot, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return fmt.Errorf("resolve git exclude file: %w", err)
	}
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(projectRoot, excludePath)
	}
	data, err := os.ReadFile(excludePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read git exclude file: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == ".crush/context/" {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return fmt.Errorf("create git exclude directory: %w", err)
	}
	f, err := os.OpenFile(excludePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open git exclude file: %w", err)
	}
	defer f.Close()
	if len(data) > 0 && data[len(data)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return fmt.Errorf("update git exclude file: %w", err)
		}
	}
	if _, err := f.WriteString("# Crush local workflow memory\n.crush/context/\n"); err != nil {
		return fmt.Errorf("update git exclude file: %w", err)
	}
	return nil
}

var workflowSlugRE = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func slugifyWorkflowName(value string) string {
	value = strings.TrimSpace(value)
	value = workflowSlugRE.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-._")
	if value == "" {
		return "local"
	}
	if len(value) > 80 {
		value = strings.Trim(value[:80], "-._")
	}
	if value == "" {
		return "local"
	}
	return value
}
