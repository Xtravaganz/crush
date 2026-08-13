package lsp

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestResolveJavaScriptProjectLSPTypeScript6(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, filepath.Join(root, "package.json"), `{
		"devDependencies": {"typescript": "^6.0.2"}
	}`)
	writePackageJSON(t, filepath.Join(root, "node_modules", "typescript", "package.json"), `{"version":"6.0.2"}`)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "node_modules", "typescript", "lib"), 0o755))
	writeExecutable(t, filepath.Join(root, "node_modules", ".bin", "typescript-language-server"))

	file := filepath.Join(root, "src", "index.ts")
	resolved, handled := resolveJavaScriptProjectLSP(file, root, missingLookPath)

	require.True(t, handled)
	require.NotNil(t, resolved)
	require.Equal(t, "typescript", resolved.Name)
	require.Equal(t, root, resolved.Root)
	require.Equal(t, filepath.Join(root, "node_modules", ".bin", executableName("typescript-language-server")), resolved.Config.Command)
	require.Equal(t, []string{"--stdio"}, resolved.Config.Args)
	require.Equal(t, filepath.Join(root, "node_modules", "typescript", "lib"), resolved.Config.InitOptions["tsserver"].(map[string]any)["path"])
}

func TestResolveJavaScriptProjectLSPTypeScript7Native(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, filepath.Join(root, "package.json"), `{
		"devDependencies": {"typescript": "^7.0.1"}
	}`)
	writePackageJSON(t, filepath.Join(root, "node_modules", "typescript", "package.json"), `{"version":"7.0.1"}`)
	writeExecutable(t, filepath.Join(root, "node_modules", ".bin", "tsc"))

	resolved, handled := resolveJavaScriptProjectLSP(filepath.Join(root, "src", "index.ts"), root, missingLookPath)

	require.True(t, handled)
	require.NotNil(t, resolved)
	require.Equal(t, filepath.Join(root, "node_modules", ".bin", executableName("tsc")), resolved.Config.Command)
	require.Equal(t, []string{"--lsp", "--stdio"}, resolved.Config.Args)
	require.Empty(t, resolved.Config.InitOptions)
	require.Contains(t, resolved.Reason, "TypeScript 7")
}

func TestResolveJavaScriptProjectLSPTypeScript7DoesNotUseGlobalTSC(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, filepath.Join(root, "package.json"), `{
		"devDependencies": {"typescript": "^7.0.1"}
	}`)
	writePackageJSON(t, filepath.Join(root, "node_modules", "typescript", "package.json"), `{"version":"7.0.1"}`)

	lookups := 0
	resolved, handled := resolveJavaScriptProjectLSP(
		filepath.Join(root, "src", "index.ts"),
		root,
		func(string) (string, error) {
			lookups++
			return "/usr/local/bin/tsc", nil
		},
	)

	require.True(t, handled)
	require.Nil(t, resolved)
	require.Equal(t, 0, lookups)
}

func TestResolveJavaScriptProjectLSPSvelte5(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, filepath.Join(root, "package.json"), `{
		"devDependencies": {
			"svelte": "^5.55.0",
			"@sveltejs/kit": "^2.57.0",
			"typescript": "^6.0.2"
		}
	}`)
	writePackageJSON(t, filepath.Join(root, "node_modules", "svelte", "package.json"), `{"version":"5.55.2"}`)
	writePackageJSON(t, filepath.Join(root, "node_modules", "@sveltejs", "kit", "package.json"), `{"version":"2.57.0"}`)
	writeExecutable(t, filepath.Join(root, "node_modules", ".bin", "svelteserver"))

	resolved, handled := resolveJavaScriptProjectLSP(filepath.Join(root, "src", "routes", "+page.svelte"), root, missingLookPath)

	require.True(t, handled)
	require.NotNil(t, resolved)
	require.Equal(t, "svelte", resolved.Name)
	require.Equal(t, []string{"--stdio"}, resolved.Config.Args)
	require.Equal(t, []string{"svelte"}, resolved.Config.FileTypes)
	require.Contains(t, resolved.Reason, "Svelte 5")
	require.Contains(t, resolved.Reason, "SvelteKit 2")
}

func TestResolveJavaScriptProjectLSPSvelteTypeScript7UsesCompatibilityTypeScript6(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, filepath.Join(root, "package.json"), `{
		"devDependencies": {
			"svelte": "^5.55.0",
			"@sveltejs/kit": "^2.57.0",
			"typescript": "^7.0.1",
			"@typescript/typescript6": "^6.0.2"
		}
	}`)
	writePackageJSON(t, filepath.Join(root, "node_modules", "typescript", "package.json"), `{"version":"7.0.1"}`)
	writePackageJSON(t, filepath.Join(root, "node_modules", "@typescript", "typescript6", "package.json"), `{"version":"6.0.2"}`)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "node_modules", "@typescript", "typescript6", "lib"), 0o755))
	writeExecutable(t, filepath.Join(root, "node_modules", ".bin", "typescript-language-server"))

	resolved, handled := resolveJavaScriptProjectLSP(filepath.Join(root, "src", "routes", "+page.server.ts"), root, missingLookPath)

	require.True(t, handled)
	require.NotNil(t, resolved)
	require.Equal(t, []string{"--stdio"}, resolved.Config.Args)
	require.Equal(
		t,
		filepath.Join(root, "node_modules", "@typescript", "typescript6", "lib"),
		resolved.Config.InitOptions["tsserver"].(map[string]any)["path"],
	)
	require.Contains(t, resolved.Reason, "TypeScript 6")
}

func TestResolveJavaScriptProjectLSPSvelteTypeScript7WithoutCompatibilityIsHandledButSkipped(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, filepath.Join(root, "package.json"), `{
		"devDependencies": {
			"svelte": "^5.55.0",
			"typescript": "^7.0.1"
		}
	}`)
	writePackageJSON(t, filepath.Join(root, "node_modules", "typescript", "package.json"), `{"version":"7.0.1"}`)
	writeExecutable(t, filepath.Join(root, "node_modules", ".bin", "typescript-language-server"))

	resolved, handled := resolveJavaScriptProjectLSP(filepath.Join(root, "src", "lib", "foo.ts"), root, missingLookPath)

	require.True(t, handled)
	require.Nil(t, resolved)
}

func TestDetectJavaScriptProjectUsesNearestPackageRootAndParentNodeModules(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "apps", "web")
	writePackageJSON(t, filepath.Join(workspace, "package.json"), `{"private":true}`)
	writePackageJSON(t, filepath.Join(app, "package.json"), `{
		"devDependencies": {"svelte":"^5.0.0", "typescript":"^6.0.0"}
	}`)
	writePackageJSON(t, filepath.Join(workspace, "node_modules", "typescript", "package.json"), `{"version":"6.0.2"}`)
	require.NoError(t, os.MkdirAll(filepath.Join(workspace, "node_modules", "typescript", "lib"), 0o755))
	writeExecutable(t, filepath.Join(workspace, "node_modules", ".bin", "typescript-language-server"))

	file := filepath.Join(app, "src", "lib", "foo.ts")
	project, ok := detectJavaScriptProject(file, workspace)
	require.True(t, ok)
	require.Equal(t, app, project.Root)
	require.Equal(t, 6, project.TypeScriptMajor)

	resolved, handled := resolveJavaScriptProjectLSP(file, workspace, missingLookPath)
	require.True(t, handled)
	require.NotNil(t, resolved)
	require.Equal(t, app, resolved.Root)
	require.Equal(t, filepath.Join(workspace, "node_modules", ".bin", executableName("typescript-language-server")), resolved.Config.Command)
}

func TestProjectClientNameIncludesNestedProjectRoot(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "apps", "web")
	require.Equal(t, "typescript", projectClientName("typescript", workspace, workspace))
	require.Equal(t, "typescript@apps/web", projectClientName("typescript", root, workspace))
}

func TestProjectLSPUserOverride(t *testing.T) {
	configured := map[string]config.LSPConfig{
		"typescript": {Disabled: true},
		"custom":     {Command: "svelteserver"},
	}
	require.True(t, projectLSPUserOverride(configured, "typescript"))
	require.True(t, projectLSPUserOverride(configured, "svelte"))
	require.False(t, projectLSPUserOverride(map[string]config.LSPConfig{"gopls": {Command: "gopls"}}, "typescript"))
}

func writePackageJSON(t *testing.T, path, data string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	path = filepath.Join(filepath.Dir(path), executableName(filepath.Base(path)))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), mode))
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".cmd"
	}
	return name
}

func missingLookPath(string) (string, error) {
	return "", errors.New("not found")
}
