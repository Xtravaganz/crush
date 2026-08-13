package lsp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/crush/internal/config"
)

type javaScriptProject struct {
	Root string

	TypeScriptMajor int
	SvelteMajor     int
	SvelteKitMajor  int

	HasTypeScript bool
	HasSvelte     bool
	HasSvelteKit  bool

	TypeScriptLib  string
	TypeScript6Lib string
}

type resolvedProjectLSP struct {
	Name   string
	Root   string
	Config config.LSPConfig
	Reason string
}

type packageManifest struct {
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Dependencies     map[string]string `json:"dependencies"`
	DevDependencies  map[string]string `json:"devDependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
}

var versionMajorPattern = regexp.MustCompile(`(?:^|[^0-9])([0-9]+)\.`)

func resolveJavaScriptProjectLSP(filePath, workDir string, lookPath func(string) (string, error)) (*resolvedProjectLSP, bool) {
	kind := javaScriptFileKind(filePath)
	if kind == "" {
		return nil, false
	}

	project, ok := detectJavaScriptProject(filePath, workDir)
	if !ok {
		return nil, false
	}

	switch kind {
	case "svelte":
		if !project.HasSvelte {
			return nil, false
		}
		command, ok := findProjectExecutable(project.Root, workDir, "svelteserver", lookPath)
		if !ok {
			slog.Debug("Svelte project detected but svelteserver was not found", "root", project.Root)
			return nil, true
		}
		return &resolvedProjectLSP{
			Name: "svelte",
			Root: project.Root,
			Config: config.LSPConfig{
				Command:   command,
				Args:      []string{"--stdio"},
				FileTypes: []string{"svelte"},
			},
			Reason: describeSvelteProject(project),
		}, true

	case "typescript":
		// Without a concrete installed/declared TypeScript version there is no
		// safe version-aware choice to make. Leave the existing catalog fallback
		// untouched in that case.
		if project.TypeScriptMajor == 0 {
			return nil, false
		}

		// TypeScript 7 ships a native LSP. Embedded-language frameworks such
		// as Svelte still need the TypeScript 6 service API for editor support,
		// so keep Svelte/SvelteKit on the legacy server path for now.
		if project.TypeScriptMajor >= 7 && !project.HasSvelte {
			// The native command is version-sensitive. Never substitute an
			// unrelated global tsc for a repository that declared TypeScript 7.
			command, ok := findLocalProjectExecutable(project.Root, workDir, "tsc")
			if !ok {
				slog.Debug("TypeScript 7 project detected but project tsc was not found", "root", project.Root)
				return nil, true
			}
			return &resolvedProjectLSP{
				Name: "typescript",
				Root: project.Root,
				Config: config.LSPConfig{
					Command:   command,
					Args:      []string{"--lsp", "--stdio"},
					FileTypes: javaScriptTypeScriptFileTypes(),
				},
				Reason: fmt.Sprintf("TypeScript %d native LSP", project.TypeScriptMajor),
			}, true
		}

		command, ok := findProjectExecutable(project.Root, workDir, "typescript-language-server", lookPath)
		if !ok {
			slog.Debug("TypeScript project detected but typescript-language-server was not found", "root", project.Root)
			return nil, true
		}

		tsLib := project.TypeScriptLib
		if project.HasSvelte && project.TypeScriptMajor >= 7 {
			tsLib = project.TypeScript6Lib
			if tsLib == "" {
				slog.Warn(
					"Svelte project uses TypeScript 7 but no TypeScript 6 compatibility install was found; skipping TypeScript LSP",
					"root", project.Root,
				)
				return nil, true
			}
		}
		if project.TypeScriptMajor <= 6 && tsLib == "" {
			slog.Debug("TypeScript <=6 project detected but local TypeScript service was not installed", "root", project.Root)
			return nil, true
		}

		initOptions := map[string]any{}
		if tsLib != "" {
			initOptions["tsserver"] = map[string]any{"path": tsLib}
		}

		reason := fmt.Sprintf("TypeScript %d via typescript-language-server", project.TypeScriptMajor)
		if project.HasSvelte {
			reason = fmt.Sprintf("Svelte-compatible TypeScript %d editor service", effectiveTypeScriptMajor(project, tsLib))
		}
		return &resolvedProjectLSP{
			Name: "typescript",
			Root: project.Root,
			Config: config.LSPConfig{
				Command:     command,
				Args:        []string{"--stdio"},
				FileTypes:   javaScriptTypeScriptFileTypes(),
				InitOptions: initOptions,
			},
			Reason: reason,
		}, true
	}

	return nil, false
}

func effectiveTypeScriptMajor(project *javaScriptProject, tsLib string) int {
	if project.HasSvelte && project.TypeScriptMajor >= 7 && tsLib == project.TypeScript6Lib {
		return 6
	}
	return project.TypeScriptMajor
}

func javaScriptFileKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svelte":
		return "svelte"
	case ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs":
		return "typescript"
	default:
		return ""
	}
}

func javaScriptTypeScriptFileTypes() []string {
	return []string{"ts", "tsx", "mts", "cts", "js", "jsx", "mjs", "cjs"}
}

func detectJavaScriptProject(filePath, workDir string) (*javaScriptProject, bool) {
	root, ok := nearestJavaScriptProjectRoot(filePath, workDir)
	if !ok {
		return nil, false
	}

	project := &javaScriptProject{Root: root}
	manifest, manifestPath, _ := nearestPackageManifest(root, workDir)
	if manifest != nil {
		project.HasTypeScript = packageDeclared(manifest, "typescript") || hasConfigFile(root, "tsconfig.json", "jsconfig.json")
		project.HasSvelte = packageDeclared(manifest, "svelte") || hasSvelteConfig(root)
		project.HasSvelteKit = packageDeclared(manifest, "@sveltejs/kit")

		project.TypeScriptMajor = packageDeclaredMajor(manifest, "typescript")
		project.SvelteMajor = packageDeclaredMajor(manifest, "svelte")
		project.SvelteKitMajor = packageDeclaredMajor(manifest, "@sveltejs/kit")

		if manifestPath != "" && project.Root == "" {
			project.Root = filepath.Dir(manifestPath)
		}
	} else {
		project.HasTypeScript = hasConfigFile(root, "tsconfig.json", "jsconfig.json")
		project.HasSvelte = hasSvelteConfig(root)
	}

	if version, pkgDir, ok := findInstalledPackage(root, workDir, "typescript"); ok {
		project.HasTypeScript = true
		if major := majorVersion(version); major > 0 {
			project.TypeScriptMajor = major
		}
		if project.TypeScriptMajor > 0 && project.TypeScriptMajor <= 6 {
			project.TypeScriptLib = filepath.Join(pkgDir, "lib")
		}
	}
	if version, _, ok := findInstalledPackage(root, workDir, "svelte"); ok {
		project.HasSvelte = true
		if major := majorVersion(version); major > 0 {
			project.SvelteMajor = major
		}
	}
	if version, _, ok := findInstalledPackage(root, workDir, "@sveltejs/kit"); ok {
		project.HasSvelteKit = true
		if major := majorVersion(version); major > 0 {
			project.SvelteKitMajor = major
		}
	}

	// During the TS7 transition Svelte tooling may install a parallel TS6
	// compatibility package. Support the official package name as well as
	// npm aliases whose installed package version is still 6.x.
	if version, pkgDir, ok := findInstalledPackage(root, workDir, "@typescript/typescript6"); ok && majorVersion(version) == 6 {
		project.TypeScript6Lib = filepath.Join(pkgDir, "lib")
	}
	if project.TypeScriptLib != "" && project.TypeScriptMajor <= 6 {
		project.TypeScript6Lib = project.TypeScriptLib
	}

	return project, project.HasTypeScript || project.HasSvelte || project.HasSvelteKit
}

func nearestJavaScriptProjectRoot(filePath, workDir string) (string, bool) {
	absWorkDir, err := filepath.Abs(workDir)
	if err == nil {
		workDir = absWorkDir
	}

	start := filePath
	if info, err := os.Stat(start); err == nil && !info.IsDir() {
		start = filepath.Dir(start)
	} else if filepath.Ext(start) != "" {
		start = filepath.Dir(start)
	}
	if abs, err := filepath.Abs(start); err == nil {
		start = abs
	}

	for dir := start; withinDir(dir, workDir); dir = filepath.Dir(dir) {
		if isJavaScriptProjectRoot(dir) {
			return dir, true
		}
		if samePath(dir, workDir) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", false
}

func isJavaScriptProjectRoot(dir string) bool {
	if fileExists(filepath.Join(dir, "package.json")) ||
		fileExists(filepath.Join(dir, "tsconfig.json")) ||
		fileExists(filepath.Join(dir, "jsconfig.json")) ||
		hasSvelteConfig(dir) {
		return true
	}
	return false
}

func hasSvelteConfig(dir string) bool {
	matches, _ := filepath.Glob(filepath.Join(dir, "svelte.config.*"))
	return len(matches) > 0
}

func hasConfigFile(dir string, names ...string) bool {
	for _, name := range names {
		if fileExists(filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

func nearestPackageManifest(start, workDir string) (*packageManifest, string, error) {
	for dir := start; withinDir(dir, workDir); dir = filepath.Dir(dir) {
		path := filepath.Join(dir, "package.json")
		manifest, err := readPackageManifest(path)
		if err == nil {
			return manifest, path, nil
		}
		if !os.IsNotExist(err) {
			return nil, "", err
		}
		if samePath(dir, workDir) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return nil, "", os.ErrNotExist
}

func readPackageManifest(path string) (*packageManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest packageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func packageDeclared(manifest *packageManifest, name string) bool {
	_, ok := packageVersionSpec(manifest, name)
	return ok
}

func packageDeclaredMajor(manifest *packageManifest, name string) int {
	spec, ok := packageVersionSpec(manifest, name)
	if !ok {
		return 0
	}
	return majorVersion(spec)
}

func packageVersionSpec(manifest *packageManifest, name string) (string, bool) {
	for _, deps := range []map[string]string{manifest.Dependencies, manifest.DevDependencies, manifest.PeerDependencies} {
		if version, ok := deps[name]; ok {
			return version, true
		}
	}
	return "", false
}

func majorVersion(version string) int {
	match := versionMajorPattern.FindStringSubmatch(version)
	if len(match) != 2 {
		return 0
	}
	major, _ := strconv.Atoi(match[1])
	return major
}

func findInstalledPackage(start, workDir, packageName string) (version, packageDir string, ok bool) {
	for dir := start; withinDir(dir, workDir); dir = filepath.Dir(dir) {
		pkgDir := filepath.Join(dir, "node_modules", filepath.FromSlash(packageName))
		manifest, err := readPackageManifest(filepath.Join(pkgDir, "package.json"))
		if err == nil {
			return manifest.Version, pkgDir, true
		}
		if samePath(dir, workDir) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", "", false
}

func findLocalProjectExecutable(start, workDir, name string) (string, bool) {
	for dir := start; withinDir(dir, workDir); dir = filepath.Dir(dir) {
		for _, candidate := range executableCandidates(filepath.Join(dir, "node_modules", ".bin", name)) {
			if executableFile(candidate) {
				return candidate, true
			}
		}
		if samePath(dir, workDir) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", false
}

func findProjectExecutable(start, workDir, name string, lookPath func(string) (string, error)) (string, bool) {
	if path, ok := findLocalProjectExecutable(start, workDir, name); ok {
		return path, true
	}

	if lookPath == nil {
		lookPath = exec.LookPath
	}
	path, err := lookPath(name)
	return path, err == nil
}

func executableCandidates(path string) []string {
	if runtime.GOOS != "windows" {
		return []string{path}
	}
	return []string{path + ".exe", path + ".cmd", path + ".bat", path}
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

func withinDir(path, root string) bool {
	path, err1 := filepath.Abs(path)
	root, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func describeSvelteProject(project *javaScriptProject) string {
	parts := []string{"Svelte language server"}
	if project.SvelteMajor > 0 {
		parts = append(parts, fmt.Sprintf("Svelte %d", project.SvelteMajor))
	}
	if project.HasSvelteKit {
		if project.SvelteKitMajor > 0 {
			parts = append(parts, fmt.Sprintf("SvelteKit %d", project.SvelteKitMajor))
		} else {
			parts = append(parts, "SvelteKit")
		}
	}
	return strings.Join(parts, ", ")
}

func projectClientName(name, root, workDir string) string {
	if samePath(root, workDir) {
		return name
	}
	rel, err := filepath.Rel(workDir, root)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return name
	}
	return name + "@" + filepath.ToSlash(rel)
}

func isJavaScriptServer(command string) bool {
	base := strings.ToLower(filepath.Base(command))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return strings.Contains(base, "typescript-language-server") ||
		strings.Contains(base, "svelteserver") ||
		strings.Contains(base, "svelte-language-server") ||
		base == "tsserver" ||
		base == "tsgo" ||
		base == "tsc"
}

func projectLSPUserOverride(configured map[string]config.LSPConfig, kind string) bool {
	for name, cfg := range configured {
		tokens := strings.ToLower(name + " " + cfg.Command)
		switch kind {
		case "svelte":
			if strings.Contains(tokens, "svelte") {
				return true
			}
		case "typescript":
			if strings.Contains(tokens, "typescript") ||
				strings.Contains(tokens, "tsserver") ||
				strings.Contains(tokens, "tsgo") {
				return true
			}
		}
	}
	return false
}
