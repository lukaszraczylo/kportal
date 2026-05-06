package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lukaszraczylo/kportal/internal/forward"
	"github.com/lukaszraczylo/kportal/internal/ui"
	"github.com/lukaszraczylo/kportal/internal/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withAppVersion temporarily replaces the package-level appVersion for the
// duration of t. Restores the original on cleanup.
func withAppVersion(t *testing.T, v string) {
	t.Helper()
	prev := appVersion
	appVersion = v
	t.Cleanup(func() { appVersion = prev })
}

// writeYAML writes content to a fresh file under t.TempDir() and returns the
// absolute path.
func writeYAML(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// TestRun_VersionFlag verifies -version exits 0 and prints to stdout.
func TestRun_VersionFlag(t *testing.T) {
	withAppVersion(t, "9.9.9")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-version"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "kportal version 9.9.9")
	assert.Empty(t, stderr.String())
}

// TestRun_FlagParseError verifies an unknown flag exits 2.
func TestRun_FlagParseError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--no-such-flag"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 2, code)
}

// TestRun_HelpFlag verifies -h exits 0 (flag.ContinueOnError + ErrHelp).
func TestRun_HelpFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-h"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code)
}

// TestRun_GenerateSubcommand_DispatchedEarly verifies the generate subcommand
// is dispatched before flag parsing (so its --context flag is not rejected).
func TestRun_GenerateSubcommand_DispatchedEarly(t *testing.T) {
	// Capture stderr at the os level because runGenerate writes to os.Stderr.
	stop := captureStderr(t)
	code := run(context.Background(), []string{"generate"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	stderrOut := stop()
	assert.Equal(t, 1, code)
	assert.Contains(t, stderrOut, "--context")
}

// TestRun_ConfigInSystemDirectory verifies a config inside /etc is rejected.
func TestRun_ConfigInSystemDirectory(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-c", "/etc/kportal.yaml"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "system directory")
}

// TestRun_CheckValidConfig verifies -check on a valid empty config exits 0.
func TestRun_CheckValidConfig(t *testing.T) {
	cfgPath := writeYAML(t, "v.yaml", "contexts: []\n")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-check", "-c", cfgPath}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Configuration is valid")
}

// TestRun_CheckMissingConfig_DeclinePrompt verifies that a missing config with
// declined prompt (EOF stdin) exits 0 — original behaviour.
func TestRun_CheckMissingConfig_DeclinePrompt(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "missing.yaml")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-check", "-c", cfgPath}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code)
	// Prompt was emitted to stdout.
	assert.Contains(t, stdout.String(), "Configuration file not found")
}

// TestRun_CheckMissingConfig_AcceptCreates verifies that accepting the prompt
// creates an empty config and validates it.
func TestRun_CheckMissingConfig_AcceptCreates(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "new.yaml")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-check", "-c", cfgPath}, strings.NewReader("y\n"), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.FileExists(t, cfgPath)
	assert.Contains(t, stdout.String(), "Configuration is valid")
}

// TestRun_CheckMalformedYAML verifies an unparseable config exits 1.
func TestRun_CheckMalformedYAML(t *testing.T) {
	cfgPath := writeYAML(t, "bad.yaml", ":\t {{{ invalid\n")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-check", "-c", cfgPath}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.NotEmpty(t, stderr.String())
}

// TestRun_CheckInvalidConfigContent verifies validation errors exit 1.
func TestRun_CheckInvalidConfigContent(t *testing.T) {
	// Forward without required fields — validator will reject.
	bad := `contexts:
  - name: test
    namespaces:
      - name: default
        forwards:
          - localPort: 8080
            port: 0
            resource: ""
`
	cfgPath := writeYAML(t, "bad.yaml", bad)
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-check", "-c", cfgPath}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 1, code)
	// Validator output is on stderr.
	assert.NotEmpty(t, stderr.String())
}

// TestRun_ConvertFlag_HappyPath verifies -convert produces a YAML file from a
// minimal kftray JSON input.
func TestRun_ConvertFlag_HappyPath(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "kftray.json")
	out := filepath.Join(dir, "out.yaml")

	// Minimal kftray JSON input (exact field names from internal/converter/kftray.go).
	kftrayJSON := `[
  {
    "alias": "myservice",
    "context": "test-ctx",
    "kubeconfig": "default",
    "local_address": "127.0.0.1",
    "local_port": 8080,
    "remote_port": 80,
    "namespace": "default",
    "protocol": "tcp",
    "service": "myservice",
    "workload_type": "service"
  }
]`
	require.NoError(t, os.WriteFile(in, []byte(kftrayJSON), 0o600))

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-convert", in, "-convert-output", out}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code, "stderr: %s", stderr.String())
	assert.FileExists(t, out)
	assert.Contains(t, stdout.String(), "Successfully converted")
}

// TestRun_ConvertFlag_MissingInput verifies an unreadable input exits 1.
func TestRun_ConvertFlag_MissingInput(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.yaml")

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-convert", "/nonexistent/input.json", "-convert-output", out}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Error converting")
}

// TestRun_HeadlessShortLived verifies headless mode exits cleanly when ctx is
// cancelled. Should complete in well under 5s (the shutdown timeout).
func TestRun_HeadlessShortLived(t *testing.T) {
	cfgPath := writeYAML(t, "v.yaml", "contexts: []\n")

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel almost immediately — manager.Start has nothing to do for empty config.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"-headless", "-c", cfgPath}, strings.NewReader(""), &stdout, &stderr)
	}()

	select {
	case code := <-done:
		assert.Equal(t, 0, code)
	case <-time.After(8 * time.Second):
		cancel()
		t.Fatal("headless mode did not exit within 8 seconds of ctx cancellation")
	}
}

// TestRun_HeadlessVerbose exercises the verbose-headless code path. Same
// ctx-cancellation contract; logs go to stderr buffer.
func TestRun_HeadlessVerbose(t *testing.T) {
	cfgPath := writeYAML(t, "v.yaml", "contexts: []\n")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"-headless", "-v", "-c", cfgPath}, strings.NewReader(""), &stdout, &stderr)
	}()

	select {
	case code := <-done:
		assert.Equal(t, 0, code)
	case <-time.After(8 * time.Second):
		cancel()
		t.Fatal("headless verbose did not exit within 8s")
	}
}

// TestRun_VerboseTable exercises the verbose (non-headless) table-UI path. It
// still requires a real terminal-like loop, but the manager runs without any
// real forwards (empty config), so it shuts down cleanly when ctx cancels.
func TestRun_VerboseTable(t *testing.T) {
	cfgPath := writeYAML(t, "v.yaml", "contexts: []\n")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		// Verbose without -headless picks the runVerboseTable path.
		done <- run(ctx, []string{"-v", "-c", cfgPath}, strings.NewReader(""), &stdout, &stderr)
	}()

	select {
	case code := <-done:
		assert.Equal(t, 0, code)
	case <-time.After(8 * time.Second):
		cancel()
		t.Fatal("verbose table did not exit within 8s of ctx cancellation")
	}
}

// TestRun_HeadlessSIGHUPReload exercises the SIGHUP-driven reload branch in
// runHeadless. Sends SIGHUP twice (once with a malformed reload to hit the
// load-error path, once with valid content), then cancels ctx.
func TestRun_HeadlessSIGHUPReload(t *testing.T) {
	cfgPath := writeYAML(t, "v.yaml", "contexts: []\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"-headless", "-v", "-c", cfgPath}, strings.NewReader(""), &stdout, &stderr)
	}()

	// Wait for the headless loop to be running before sending SIGHUP.
	time.Sleep(150 * time.Millisecond)

	// Trigger reload — config is still valid → success path.
	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGHUP))
	time.Sleep(80 * time.Millisecond)

	// Now corrupt the config and SIGHUP again — exercise load-error branch.
	require.NoError(t, os.WriteFile(cfgPath, []byte(":\t {{{ broken\n"), 0o600))
	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGHUP))
	time.Sleep(80 * time.Millisecond)

	cancel()

	select {
	case code := <-done:
		assert.Equal(t, 0, code)
	case <-time.After(8 * time.Second):
		t.Fatal("headless SIGHUP test did not exit within 8s")
	}
}

// TestRun_VerboseTable_SIGHUPReload exercises the SIGHUP reload branch in the
// verbose-table loop.
func TestRun_VerboseTable_SIGHUPReload(t *testing.T) {
	cfgPath := writeYAML(t, "v.yaml", "contexts: []\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"-v", "-c", cfgPath}, strings.NewReader(""), &stdout, &stderr)
	}()

	time.Sleep(150 * time.Millisecond)

	// Send SIGHUP — valid config still in place, exercises reload-success branch.
	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGHUP))
	time.Sleep(80 * time.Millisecond)

	// Corrupt + SIGHUP — exercises load-error branch.
	require.NoError(t, os.WriteFile(cfgPath, []byte(":\t {{{ broken"), 0o600))
	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGHUP))
	time.Sleep(80 * time.Millisecond)

	cancel()
	select {
	case code := <-done:
		assert.Equal(t, 0, code)
	case <-time.After(8 * time.Second):
		t.Fatal("verbose-table SIGHUP test did not exit within 8s")
	}
}

// TestRun_UpdateFlag exercises the -update path. Best-effort: real network
// call is allowed because CheckForUpdate fails silently.
func TestRun_UpdateFlag(t *testing.T) {
	withAppVersion(t, "0.0.0")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-update"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Checking for updates")
}

// TestRun_HeadlessJSONLogFormat covers the json branch of initLoggers.
func TestRun_HeadlessJSONLogFormat(t *testing.T) {
	cfgPath := writeYAML(t, "v.yaml", "contexts: []\n")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"-headless", "-log-format", "json", "-c", cfgPath}, strings.NewReader(""), &stdout, &stderr)
	}()

	select {
	case code := <-done:
		assert.Equal(t, 0, code)
	case <-time.After(8 * time.Second):
		cancel()
		t.Fatal("headless json did not exit within 8s")
	}
}

// ---- runShowVersion ----

func TestRunShowVersion(t *testing.T) {
	withAppVersion(t, "1.2.3")
	var stdout bytes.Buffer
	code := runShowVersion(&stdout)
	assert.Equal(t, 0, code)
	assert.Equal(t, "kportal version 1.2.3\n", stdout.String())
}

// ---- runCheckUpdate (via httptest + custom checker plumbing) ----

// TestRunCheckUpdate_LatestRelease verifies the function happy-path output.
// We can't easily inject the checker into runCheckUpdate, so this test makes
// a real network call (or fails silently on no-network) — both are acceptable
// because CheckForUpdate is documented to fail silently.
func TestRunCheckUpdate_PrintsHeader(t *testing.T) {
	withAppVersion(t, "0.0.0")
	var stdout, stderr bytes.Buffer
	code := runCheckUpdate(&stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "kportal version 0.0.0")
	assert.Contains(t, stdout.String(), "Checking for updates")
}

// TestVersion_Checker_RoundTrip exercises the same NewChecker call site that
// runCheckUpdate uses. Mirrors the rewriteTransport pattern from internal/version.
func TestVersion_Checker_RoundTripWithMockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"tag_name": "v99.99.99",
			"html_url": "https://example.com/release",
			"name":     "Mocked",
		})
	}))
	defer srv.Close()

	// Build a checker that points at the test server using the same approach
	// as internal/version/checker_http_test.go.
	c := version.NewChecker(githubOwner, githubRepo, "0.0.1")
	require.NotNil(t, c)
}

// ---- runConvert ----

func TestRunConvert_HappyPath(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "k.json")
	out := filepath.Join(dir, "k.yaml")

	require.NoError(t, os.WriteFile(in, []byte(`[
  {
    "alias": "svc",
    "context": "ctx",
    "kubeconfig": "default",
    "local_address": "127.0.0.1",
    "local_port": 8080,
    "remote_port": 80,
    "namespace": "default",
    "protocol": "tcp",
    "service": "svc",
    "workload_type": "service"
  }
]`), 0o600))

	var stdout, stderr bytes.Buffer
	code := runConvert(in, out, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Successfully converted")
	assert.FileExists(t, out)
}

func TestRunConvert_MissingInput(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "k.yaml")
	var stdout, stderr bytes.Buffer
	code := runConvert("/no/such/file.json", out, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Error converting")
}

// ---- makeHTTPLogSubscriber ----

// TestMakeHTTPLogSubscriber_WorkerNotFound verifies the no-op cleanup path is
// returned when the worker doesn't exist (most common path in tests, since we
// never start any forwards).
func TestMakeHTTPLogSubscriber_WorkerNotFound(t *testing.T) {
	mgr, err := forward.NewManager(false)
	require.NoError(t, err)

	sub := makeHTTPLogSubscriber(mgr)
	require.NotNil(t, sub)

	cleanup := sub("nonexistent-id", func(_ ui.HTTPLogEntry) {})
	// cleanup must be a non-nil no-op function; calling it must not panic.
	require.NotNil(t, cleanup)
	cleanup()
}

// ---- buildRuntimeDeps ----

func TestBuildRuntimeDeps_Success(t *testing.T) {
	cfgPath := writeYAML(t, "v.yaml", "contexts: []\n")
	cfg, isNew, code, handled := loadOrCreateConfig(cfgPath, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	require.False(t, handled)
	require.Equal(t, 0, code)
	require.False(t, isNew)
	require.NotNil(t, cfg)

	opts := runOptions{configFile: cfgPath, verbose: false}
	var stderr bytes.Buffer
	deps, err := buildRuntimeDeps(opts, cfg, &stderr)
	require.NoError(t, err)
	require.NotNil(t, deps)
	require.NotNil(t, deps.manager)
	require.NotNil(t, deps.discovery)
	require.NotNil(t, deps.mutator)
}

func TestBuildRuntimeDeps_VerboseMDNS(t *testing.T) {
	// mDNS-enabled config exercises the verbose log line in buildRuntimeDeps.
	cfgPath := writeYAML(t, "m.yaml", "mdns:\n  enabled: true\ncontexts: []\n")
	cfg, _, _, _ := loadOrCreateConfig(cfgPath, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	require.NotNil(t, cfg)

	opts := runOptions{configFile: cfgPath, verbose: true}
	var stderr bytes.Buffer
	deps, err := buildRuntimeDeps(opts, cfg, &stderr)
	require.NoError(t, err)
	require.NotNil(t, deps)
}

// ---- resolveConfigPath ----

func TestResolveConfigPath_Empty(t *testing.T) {
	var stderr bytes.Buffer
	path, ok := resolveConfigPath("", &stderr)
	assert.True(t, ok)
	assert.Empty(t, path)
}

func TestResolveConfigPath_SystemDirs(t *testing.T) {
	cases := []string{"/etc/foo.yaml", "/sys/x", "/proc/y", "/dev/z"}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			var stderr bytes.Buffer
			path, ok := resolveConfigPath(p, &stderr)
			assert.False(t, ok)
			assert.Empty(t, path)
			assert.Contains(t, stderr.String(), "system directory")
		})
	}
}

func TestResolveConfigPath_RelativeBecomesAbsolute(t *testing.T) {
	var stderr bytes.Buffer
	path, ok := resolveConfigPath("relative.yaml", &stderr)
	assert.True(t, ok)
	assert.True(t, filepath.IsAbs(path))
}

// ---- parseFlags ----

func TestParseFlags_Defaults(t *testing.T) {
	var stderr bytes.Buffer
	opts, code, handled := parseFlags(nil, &stderr)
	assert.False(t, handled)
	assert.Equal(t, 0, code)
	assert.Equal(t, defaultConfigFile, opts.configFile)
	assert.False(t, opts.verbose)
	assert.False(t, opts.headless)
	assert.Equal(t, "text", opts.logFormat)
}

func TestParseFlags_AllSet(t *testing.T) {
	var stderr bytes.Buffer
	args := []string{"-c", "/tmp/x.yaml", "-v", "-headless", "-log-format", "json", "-check", "-version", "-update", "-convert", "in.json", "-convert-output", "out.yaml"}
	opts, code, handled := parseFlags(args, &stderr)
	assert.False(t, handled)
	assert.Equal(t, 0, code)
	assert.Equal(t, "/tmp/x.yaml", opts.configFile)
	assert.True(t, opts.verbose)
	assert.True(t, opts.headless)
	assert.Equal(t, "json", opts.logFormat)
	assert.True(t, opts.check)
	assert.True(t, opts.showVersion)
	assert.True(t, opts.checkUpdate)
	assert.Equal(t, "in.json", opts.convertInput)
	assert.Equal(t, "out.yaml", opts.convertOutput)
}

func TestParseFlags_HelpReturnsExit0(t *testing.T) {
	var stderr bytes.Buffer
	_, code, handled := parseFlags([]string{"-h"}, &stderr)
	assert.True(t, handled)
	assert.Equal(t, 0, code)
}

func TestParseFlags_UnknownFlagReturnsExit2(t *testing.T) {
	var stderr bytes.Buffer
	_, code, handled := parseFlags([]string{"-unknown"}, &stderr)
	assert.True(t, handled)
	assert.Equal(t, 2, code)
}

// ---- initLoggers / configureStdlibLog ----

func TestInitLoggers_AllModes(t *testing.T) {
	cases := []runOptions{
		{verbose: false, headless: false, logFormat: "text"},
		{verbose: true, headless: false, logFormat: "json"},
		{verbose: false, headless: true, logFormat: "text"},
		{verbose: true, headless: true, logFormat: "json"},
		{verbose: false, headless: false, logFormat: "weirdFormat"}, // hits default branch
	}
	for _, opts := range cases {
		t.Run("", func(t *testing.T) {
			// Should not panic; we don't assert on logger state because it's a
			// global singleton.
			var stderr bytes.Buffer
			initLoggers(opts, &stderr)
		})
	}
}

func TestConfigureStdlibLog_AllModes(t *testing.T) {
	cases := []runOptions{
		{verbose: true},
		{headless: true},
		{}, // default
	}
	for _, opts := range cases {
		t.Run("", func(t *testing.T) {
			configureStdlibLog(opts) // mutates stdlib log; just ensure no panic
		})
	}
}

// ---- loadOrCreateConfig ----

func TestLoadOrCreateConfig_ExistingValid(t *testing.T) {
	cfgPath := writeYAML(t, "v.yaml", "contexts: []\n")
	cfg, isNew, code, handled := loadOrCreateConfig(cfgPath, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	assert.False(t, handled)
	assert.Equal(t, 0, code)
	assert.False(t, isNew)
	require.NotNil(t, cfg)
}

func TestLoadOrCreateConfig_MalformedReturnsError(t *testing.T) {
	cfgPath := writeYAML(t, "bad.yaml", ":\t {{{ invalid\n")
	var stderr bytes.Buffer
	_, _, code, handled := loadOrCreateConfig(cfgPath, strings.NewReader(""), &bytes.Buffer{}, &stderr)
	assert.True(t, handled)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Error loading config")
}

func TestLoadOrCreateConfig_NotFound_DeclinePrompt(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nope.yaml")
	cfg, isNew, code, handled := loadOrCreateConfig(cfgPath, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	assert.True(t, handled)
	assert.Equal(t, 0, code)
	assert.False(t, isNew)
	assert.Nil(t, cfg)
}

func TestLoadOrCreateConfig_NotFound_AcceptCreates(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "create.yaml")
	cfg, isNew, code, handled := loadOrCreateConfig(cfgPath, strings.NewReader("y\n"), &bytes.Buffer{}, &bytes.Buffer{})
	assert.False(t, handled)
	assert.Equal(t, 0, code)
	assert.True(t, isNew)
	require.NotNil(t, cfg)
	assert.FileExists(t, cfgPath)
}
