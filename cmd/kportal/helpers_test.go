package main

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeKubeconfig writes a minimal kubeconfig file to dir with a single context
// named contextName and returns the path.
func fakeKubeconfig(t *testing.T, dir, contextName string) string {
	t.Helper()
	content := `apiVersion: v1
clusters:
- cluster:
    server: https://localhost:6443
  name: fake-cluster
contexts:
- context:
    cluster: fake-cluster
    namespace: default
    user: fake-user
  name: ` + contextName + `
current-context: ` + contextName + `
kind: Config
preferences: {}
users:
- name: fake-user
  user: {}
`
	path := filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}

// ---- promptCreateConfig ----

// redirectStdin replaces os.Stdin with a pipe that has the given text
// already written into it. Returns a cleanup function that restores
// the original Stdin and closes the pipe ends.
func redirectStdin(t *testing.T, text string) func() {
	t.Helper()
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = io.WriteString(w, text)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	os.Stdin = r
	return func() {
		os.Stdin = origStdin
		require.NoError(t, r.Close())
	}
}

func TestPromptCreateConfig_YesResponses(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty enter", "\n"},
		{"lowercase y", "y\n"},
		{"uppercase Y", "Y\n"}, // ToLower normalises it
		{"yes word", "yes\n"},
		{"YES word", "YES\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := redirectStdin(t, tc.input)
			defer restore()

			result := promptCreateConfig("/some/path.yaml")
			assert.True(t, result, "expected true for input %q", tc.input)
		})
	}
}

func TestPromptCreateConfig_NoResponses(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"lowercase n", "n\n"},
		{"uppercase N", "N\n"},
		{"no word", "no\n"},
		{"other text", "nope\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := redirectStdin(t, tc.input)
			defer restore()

			result := promptCreateConfig("/some/path.yaml")
			assert.False(t, result, "expected false for input %q", tc.input)
		})
	}
}

func TestPromptCreateConfig_EOFReturnsFalse(t *testing.T) {
	// Provide an empty pipe (write end immediately closed) → EOF → false.
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	require.NoError(t, w.Close()) // no data written → EOF on first read
	os.Stdin = r
	defer func() {
		os.Stdin = origStdin
		require.NoError(t, r.Close())
	}()

	result := promptCreateConfig("/some/path.yaml")
	assert.False(t, result, "EOF should return false")
}

// ---- contains ----

func TestContains_Present(t *testing.T) {
	assert.True(t, contains([]string{"a", "b", "c"}, "b"))
}

func TestContains_Absent(t *testing.T) {
	assert.False(t, contains([]string{"a", "b", "c"}, "d"))
}

func TestContains_EmptySlice(t *testing.T) {
	assert.False(t, contains([]string{}, "x"))
}

func TestContains_EmptyNeedle(t *testing.T) {
	assert.True(t, contains([]string{"", "a"}, ""))
}

// ---- resolveGenerateConfigPath ----

func TestResolveGenerateConfigPath_EmptyPath(t *testing.T) {
	path, ok := resolveGenerateConfigPath("")
	assert.False(t, ok)
	assert.Empty(t, path)
}

func TestResolveGenerateConfigPath_SystemDirs(t *testing.T) {
	sysDirs := []string{
		"/etc/passwd",
		"/sys/kernel/config",
		"/proc/cpuinfo",
		"/dev/null",
	}
	for _, d := range sysDirs {
		t.Run(d, func(t *testing.T) {
			path, ok := resolveGenerateConfigPath(d)
			assert.False(t, ok, "system path should be rejected: %s", d)
			assert.Empty(t, path)
		})
	}
}

func TestResolveGenerateConfigPath_ValidPath(t *testing.T) {
	// A relative path should be resolved to an absolute, cleaned path.
	path, ok := resolveGenerateConfigPath("relative/config.yaml")
	assert.True(t, ok)
	assert.True(t, strings.HasPrefix(path, "/"), "should be absolute")
	assert.True(t, strings.HasSuffix(path, "relative/config.yaml"))
}

func TestResolveGenerateConfigPath_AbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/kportal.yaml"
	path, ok := resolveGenerateConfigPath(configPath)
	assert.True(t, ok)
	assert.Equal(t, configPath, path)
}

// ---- runGenerate ----

// captureStderr swaps os.Stderr for a pipe and returns a function that
// restores it and returns whatever was written.
func captureStderr(t *testing.T) func() string {
	t.Helper()
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	return func() string {
		_ = w.Close()
		os.Stderr = origStderr
		var sb strings.Builder
		_, _ = io.Copy(&sb, r)
		_ = r.Close()
		return sb.String()
	}
}

func TestRunGenerate_MissingContextFlag(t *testing.T) {
	// --context is required; omitting it should return exit-code 1.
	stop := captureStderr(t)
	code := runGenerate([]string{})
	stderr := stop()
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "--context")
}

func TestRunGenerate_HelpFlag(t *testing.T) {
	// -h / --help should return exit-code 0 (flag.ContinueOnError + ErrHelp).
	stop := captureStderr(t)
	code := runGenerate([]string{"-h"})
	_ = stop()
	assert.Equal(t, 0, code)
}

func TestRunGenerate_UnknownFlag(t *testing.T) {
	// An unrecognised flag should return exit-code 1.
	stop := captureStderr(t)
	code := runGenerate([]string{"--unknown-flag=xyz"})
	_ = stop()
	assert.Equal(t, 1, code)
}

func TestRunGenerate_SystemDirConfig(t *testing.T) {
	// A config path inside a system directory should return exit-code 1.
	stop := captureStderr(t)
	code := runGenerate([]string{"--context=minikube", "--config=/etc/kportal.yaml"})
	stderr := stop()
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "system directory")
}

func TestRunGenerate_ContextNotInKubeconfig(t *testing.T) {
	// A context that does not exist in kubeconfig should return exit-code 1.
	// This relies on k8s.NewClientPool() succeeding (it reads ~/.kube/config or
	// returns an empty pool) and ListContexts() returning a set that does not
	// contain the requested name.
	tmpDir := t.TempDir()
	configPath := tmpDir + "/kportal.yaml"

	stop := captureStderr(t)
	code := runGenerate([]string{
		"--context=this-context-does-not-exist-in-any-kubeconfig-xyz",
		"--config=" + configPath,
	})
	stderr := stop()
	assert.Equal(t, 1, code)
	// Either the context was not found, OR k8s client setup failed — both are
	// valid error paths that return 1.
	assert.NotEmpty(t, stderr)
}

// TestRunGenerate_MalformedConfig verifies that a config file with invalid YAML
// causes runGenerate to return exit-code 1 before calling ui.RunGenerate.
func TestRunGenerate_MalformedConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake kubeconfig with a known context name.
	kubecfgPath := fakeKubeconfig(t, tmpDir, "test-ctx")
	t.Setenv("KUBECONFIG", kubecfgPath)

	// Write an invalid YAML config file.
	configPath := filepath.Join(tmpDir, "bad.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(":\t invalid yaml {{{\n"), 0600))

	stop := captureStderr(t)
	code := runGenerate([]string{
		"--context=test-ctx",
		"--config=" + configPath,
	})
	stderr := stop()
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "failed to load config")
}

// TestRunGenerate_ValidContextNoUI verifies runGenerate error-handling when
// ui.RunGenerate cannot open a TTY (always the case in non-interactive test
// environments). The function should return exit-code 1 and print the error.
func TestRunGenerate_ValidContextNoUI(t *testing.T) {
	tmpDir := t.TempDir()
	kubecfgPath := fakeKubeconfig(t, tmpDir, "test-ctx")
	t.Setenv("KUBECONFIG", kubecfgPath)

	// Config file does not exist — ErrConfigNotFound is acceptable; code
	// proceeds to ui.RunGenerate which fails (no TTY in tests).
	configPath := filepath.Join(tmpDir, "nonexistent.yaml")

	stop := captureStderr(t)
	code := runGenerate([]string{
		"--context=test-ctx",
		"--config=" + configPath,
	})
	stderr := stop()
	// Either the UI failed (exit 1) or — on rare CI with a TTY — it was
	// cancelled (also exit 1). Both are acceptable outcomes for this test.
	assert.Equal(t, 1, code)
	_ = stderr // error message varies by environment
}

// ---- promptCreateConfig output via bufio path ----

// TestPromptCreateConfig_PathIncludedInOutput verifies the path is printed.
func TestPromptCreateConfig_PathIncludedInOutput(t *testing.T) {
	// Capture stdout by swapping os.Stdout temporarily.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	restore := redirectStdin(t, "n\n")
	defer restore()

	_ = promptCreateConfig("/my/special/config.yaml")

	require.NoError(t, w.Close())
	os.Stdout = origStdout

	var sb strings.Builder
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
	}
	require.NoError(t, r.Close())

	assert.Contains(t, sb.String(), "/my/special/config.yaml")
}
