package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/lukaszraczylo/kportal/internal/converter"
	"github.com/lukaszraczylo/kportal/internal/forward"
	"github.com/lukaszraczylo/kportal/internal/httplog"
	"github.com/lukaszraczylo/kportal/internal/k8s"
	"github.com/lukaszraczylo/kportal/internal/logger"
	"github.com/lukaszraczylo/kportal/internal/mdns"
	"github.com/lukaszraczylo/kportal/internal/ui"
	"github.com/lukaszraczylo/kportal/internal/version"
	telemetry "github.com/lukaszraczylo/oss-telemetry"
	"k8s.io/klog/v2"
)

const (
	defaultConfigFile        = ".kportal.yaml"
	initialForwardSettleTime = 100 * time.Millisecond
	tableUpdateInterval      = 2 * time.Second

	// GitHub repository info for update checks
	githubOwner = "lukaszraczylo"
	githubRepo  = "kportal"
)

// appVersion is the build version. Set via ldflags during build:
//
//	-X main.appVersion=v1.2.3
var appVersion = "0.1.0"

// runOptions captures the parsed flag values so each mode-specific run* function
// can be invoked independently of the global flag state. Held by value because
// it's small and travels through multiple goroutines.
type runOptions struct {
	configFile    string
	logFormat     string
	convertInput  string
	convertOutput string
	contexts      []string
	verbose       bool
	headless      bool
	check         bool
	showVersion   bool
	checkUpdate   bool
}

// contextListFlag collects repeated -context values. Each value may itself be a
// comma-separated list, so "-context a -context b" and "-context a,b" are
// equivalent.
type contextListFlag []string

func (c *contextListFlag) String() string {
	if c == nil {
		return ""
	}
	return strings.Join(*c, ",")
}

func (c *contextListFlag) Set(value string) error {
	// Reject an empty value rather than silently forwarding every context -
	// "-context=$UNSET_VAR" in a script should fail loudly.
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("context name cannot be empty")
	}
	*c = append(*c, value)
	return nil
}

// fprintf is a small wrapper that suppresses the io.Writer write error. We
// route output to caller-provided writers (stdout / stderr / io.Discard /
// bytes.Buffer in tests), and a write error on any of these is non-actionable
// at this layer.
func fprintf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// fprintln is the Fprintln equivalent of fprintf.
func fprintln(w io.Writer, args ...any) {
	_, _ = fmt.Fprintln(w, args...)
}

// fprint is the Fprint equivalent of fprintf.
func fprint(w io.Writer, args ...any) {
	_, _ = fmt.Fprint(w, args...)
}

// promptCreateConfig asks the user if they want to create a new config file.
// Returns true if the user answers yes, false otherwise.
func promptCreateConfig(path string, stdin io.Reader, stdout io.Writer) bool {
	fprintf(stdout, "Configuration file not found: %s\n", path)
	fprint(stdout, "Would you like to create an empty configuration? [Y/n] ")

	reader := bufio.NewReader(stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		// EOF with data is acceptable - the user might have piped a response without trailing newline.
		if response == "" {
			return false
		}
	}

	response = strings.TrimSpace(strings.ToLower(response))
	// Empty response (just Enter) defaults to yes
	return response == "" || response == "y" || response == "yes"
}

func main() {
	os.Exit(runMain())
}

// runMain wraps the real entry point so that deferred cleanup (telemetry
// drain, signal context cancel) runs before the surrounding os.Exit.
func runMain() int {
	telemetry.Send("kportal", appVersion)
	defer telemetry.Wait(2 * time.Second)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	return run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

// run is the testable entry point. It returns the desired process exit code
// instead of calling os.Exit. ctx must be cancelled to trigger clean shutdown
// of long-running modes (headless, verbose-loop, interactive).
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// Subcommand dispatch must run BEFORE the main flag set is parsed because
	// generate and completion have their own FlagSets and must not see kportal's top-level flags.
	if len(args) >= 1 {
		switch args[0] {
		case "generate":
			return runGenerate(args[1:])
		case "completion":
			return completionCmd(args[1:])
		}
	}

	opts, code, handled := parseFlags(args, stderr)
	if handled {
		return code
	}

	// Quick-exit informational modes — these short-circuit before any cluster
	// work and never need a config file.
	if opts.showVersion {
		return runShowVersion(stdout)
	}
	if opts.checkUpdate {
		return runCheckUpdate(stdout, stderr)
	}

	// Validate config path security (block system directories, normalise to abs).
	resolvedConfig, ok := resolveConfigPath(opts.configFile, stderr)
	if !ok {
		return 1
	}
	opts.configFile = resolvedConfig

	// Initialise structured logger / klog routing. These outputs depend on mode,
	// not on -v alone (see comment block in original implementation).
	initLoggers(opts, stderr)

	// Conversion mode runs before config load — it does not need a kportal config.
	if opts.convertInput != "" {
		return runConvert(opts.convertInput, opts.convertOutput, stdout, stderr)
	}

	// Configure stdlib log destination based on mode.
	configureStdlibLog(opts)

	// Load configuration (with optional create-on-missing prompt).
	cfg, configIsNew, code, handled := loadOrCreateConfig(opts.configFile, stdin, stdout, stderr)
	if handled {
		return code
	}

	// Validate the whole configuration - including contexts that -context
	// excludes - so problems anywhere in the file are still reported. Only the
	// checks that depend on what actually runs (local ports, mDNS hostnames)
	// are scoped to the selection.
	validator := config.NewValidator()
	if errs := validator.ValidateConfigWithOpts(cfg, config.ValidateOptions{
		AllowEmpty:     configIsNew || cfg.IsEmpty(),
		ActiveContexts: opts.contexts,
	}); len(errs) > 0 {
		fprint(stderr, config.FormatValidationErrors(errs))
		return 1
	}

	// Narrow the config to the selected contexts for everything downstream.
	// Skipped for a config with no contexts at all (a file just created above),
	// where reporting an unknown context would be unhelpful.
	if len(opts.contexts) > 0 && len(cfg.Contexts) > 0 {
		selected, err := cfg.SelectContexts(opts.contexts)
		if err != nil {
			fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		cfg = selected
	}

	if opts.check {
		fprintln(stdout, "Configuration is valid")
		return 0
	}

	if opts.verbose {
		log.Printf("kportal v%s", appVersion)
		log.Printf("Loading configuration from: %s", opts.configFile)
	}

	// Build forward manager + supporting bits, shared by headless / verbose / TUI paths.
	deps, err := buildRuntimeDeps(opts, cfg, stderr)
	if err != nil {
		fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	switch {
	case opts.headless:
		return runHeadless(ctx, opts, cfg, deps, stderr)
	case opts.verbose:
		return runVerboseTable(ctx, opts, cfg, deps, stderr)
	default:
		return runInteractive(ctx, opts, cfg, deps, stderr)
	}
}

// parseFlags binds args to a fresh FlagSet and returns the parsed options.
// On parse error / -help, returns (zero, code, true) to signal the caller to
// exit immediately with the supplied code.
func parseFlags(args []string, stderr io.Writer) (runOptions, int, bool) {
	fs := flag.NewFlagSet("kportal", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var opts runOptions
	var contexts contextListFlag
	fs.Var(&contexts, "context", "Only forward the named contexts from the config file (repeatable and comma-separated; default: all)")
	fs.StringVar(&opts.configFile, "c", defaultConfigFile, "Path to configuration file")
	fs.BoolVar(&opts.verbose, "v", false, "Enable verbose logging")
	fs.BoolVar(&opts.headless, "headless", false, "Run in headless mode (no UI, for background/daemon use)")
	fs.StringVar(&opts.logFormat, "log-format", "text", "Log format: text or json")
	fs.BoolVar(&opts.check, "check", false, "Validate configuration and exit")
	fs.BoolVar(&opts.showVersion, "version", false, "Show version and exit")
	fs.BoolVar(&opts.checkUpdate, "update", false, "Check for updates and exit")
	fs.StringVar(&opts.convertInput, "convert", "", "Convert kftray JSON config to kportal YAML (provide input file path)")
	fs.StringVar(&opts.convertOutput, "convert-output", ".kportal.yaml", "Output file for converted configuration")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return opts, 0, true
		}
		return opts, 2, true
	}
	opts.contexts = config.NormalizeContextSelection(contexts)
	return opts, 0, false
}

// resolveConfigPath validates the user-supplied config path: must resolve to
// an absolute, cleaned path that is not inside a protected system directory.
func resolveConfigPath(path string, stderr io.Writer) (string, bool) {
	if path == "" {
		return "", true // empty is allowed; caller treats it as "no config"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		fprintf(stderr, "Invalid config path: %v\n", err)
		return "", false
	}
	abs = filepath.Clean(abs)
	for _, sysDir := range []string{"/etc", "/sys", "/proc", "/dev"} {
		if strings.HasPrefix(abs, sysDir) {
			fprintf(stderr, "Error: Config file cannot be in system directory: %s\n", sysDir)
			return "", false
		}
	}
	return abs, true
}

// initLoggers configures the structured logger and klog routing. Output
// destination depends on run mode (see big comment for rationale).
func initLoggers(opts runOptions, stderr io.Writer) {
	var logLevel logger.Level
	var logFmt logger.Format
	var logOutput io.Writer

	if opts.verbose {
		logLevel = logger.LevelDebug
	} else {
		logLevel = logger.LevelInfo
	}

	if opts.headless || opts.verbose {
		logOutput = stderr
	} else {
		logOutput = io.Discard
	}

	switch opts.logFormat {
	case "json":
		logFmt = logger.FormatJSON
	default:
		logFmt = logger.FormatText
	}

	logger.Init(logLevel, logFmt, logOutput)

	klog.LogToStderr(false)
	if opts.verbose {
		klogLogger := logger.New(logger.LevelDebug, logFmt, stderr)
		klog.SetOutput(logger.NewKlogWriter(klogLogger))
		logrSink := logger.NewLogrAdapter(klogLogger)
		klog.SetLogger(logr.New(logrSink))
	} else {
		klog.SetOutput(io.Discard)
		silentLogger := logger.New(logger.LevelError+1, logFmt, io.Discard)
		logrSink := logger.NewLogrAdapter(silentLogger)
		klog.SetLogger(logr.New(logrSink))
	}
}

// configureStdlibLog matches stdlib log destination to the mode. Only the TUI
// path needs total silence; daemonised modes keep stderr.
func configureStdlibLog(opts runOptions) {
	switch {
	case opts.verbose:
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	case opts.headless:
		log.SetFlags(log.LstdFlags)
	default:
		log.SetOutput(io.Discard)
		log.SetPrefix("")
		log.SetFlags(0)
	}
}

// loadOrCreateConfig loads the config, prompting to create an empty file if it
// doesn't exist. Returns (cfg, configIsNew, exitCode, handled).
func loadOrCreateConfig(configFile string, stdin io.Reader, stdout, stderr io.Writer) (*config.Config, bool, int, bool) {
	cfg, err := config.LoadConfig(configFile)
	if err == nil {
		return cfg, false, 0, false
	}
	if err != config.ErrConfigNotFound {
		fprintf(stderr, "Error loading config: %v\n", err)
		return nil, false, 1, true
	}
	if !promptCreateConfig(configFile, stdin, stdout) {
		return nil, false, 0, true
	}
	if createErr := config.CreateEmptyConfigFile(configFile); createErr != nil {
		fprintf(stderr, "Error creating config file: %v\n", createErr)
		return nil, false, 1, true
	}
	fprintf(stdout, "Created %s\n", configFile)
	fprintln(stdout, "Use 'n' in the UI to add port forwards, or edit the file manually.")
	fprintln(stdout)

	cfg, err = config.LoadConfig(configFile)
	if err != nil {
		fprintf(stderr, "Error loading config: %v\n", err)
		return nil, false, 1, true
	}
	return cfg, true, 0, false
}

// reloadFromSignal re-reads the config after SIGHUP and applies it to the
// manager. Load, validation and the -context selection all live in
// config.LoadForRuntime, shared with the file watcher, so no reload path can
// forget the selection. Failures are logged and the previous config stays
// active. onApplied, when non-nil, runs only after a successful Reload.
func reloadFromSignal(opts runOptions, manager *forward.Manager, logIt bool, onApplied func(*config.Config)) {
	newCfg, err := config.LoadForRuntime(opts.configFile, opts.contexts)
	if err != nil {
		if logIt {
			log.Printf("Failed to reload config: %v", err)
		}
		return
	}
	if err := manager.Reload(newCfg); err != nil {
		if logIt {
			log.Printf("Failed to reload: %v", err)
		}
		return
	}
	if onApplied != nil {
		onApplied(newCfg)
	}
}

// runtimeDeps bundles the long-lived objects shared by all UI modes.
type runtimeDeps struct {
	manager   *forward.Manager
	pool      *k8s.ClientPool
	discovery *k8s.Discovery
	mutator   *config.Mutator
	mdnsPub   *mdns.Publisher
}

// buildRuntimeDeps constructs the kubernetes client pool, forward manager, and
// helpers used across run modes. Returns an error only on fatal failures
// (manager creation); a missing kubeconfig is logged but allowed.
func buildRuntimeDeps(opts runOptions, cfg *config.Config, stderr io.Writer) (*runtimeDeps, error) {
	pool, err := k8s.NewClientPool()
	if err != nil {
		fprintf(stderr, "Warning: Failed to create k8s client pool: %v\n", err)
		fprintf(stderr, "Add/remove wizards will not be available\n")
	}
	discovery := k8s.NewDiscovery(pool)
	mutator := config.NewMutator(opts.configFile)
	mutator.SetActiveContexts(opts.contexts)

	manager, err := forward.NewManager(opts.verbose)
	if err != nil {
		return nil, fmt.Errorf("creating forward manager: %w", err)
	}

	pub := mdns.NewPublisher(cfg.IsMDNSEnabled())
	manager.SetMDNSPublisher(pub)
	if cfg.IsMDNSEnabled() && opts.verbose {
		log.Printf("mDNS hostname publishing enabled - aliases will be accessible via <alias>.local")
	}

	return &runtimeDeps{
		manager:   manager,
		pool:      pool,
		discovery: discovery,
		mutator:   mutator,
		mdnsPub:   pub,
	}, nil
}

// runShowVersion prints the version and exits 0.
func runShowVersion(stdout io.Writer) int {
	fprintf(stdout, "kportal version %s\n", appVersion)
	return 0
}

// runCheckUpdate checks for available updates and prints the result.
func runCheckUpdate(stdout, _ io.Writer) int {
	fprintf(stdout, "kportal version %s\n", appVersion)
	fprintln(stdout, "Checking for updates...")

	checker := version.NewChecker(githubOwner, githubRepo, appVersion)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	update := checker.CheckForUpdate(ctx)
	if update == nil {
		fprintln(stdout, "You are running the latest version.")
		return 0
	}

	fprintf(stdout, "\nUpdate available: v%s\n", update.LatestVersion)
	fprintf(stdout, "Download: %s\n", update.ReleaseURL)
	fprintln(stdout, "\nTo update, download the latest release from the URL above")
	fprintln(stdout, "or use your package manager (e.g., 'brew upgrade kportal').")
	return 0
}

// runConvert converts a kftray JSON file to a kportal YAML config.
func runConvert(input, output string, stdout, stderr io.Writer) int {
	if err := converter.ConvertKFTrayToKPortal(input, output); err != nil {
		fprintf(stderr, "Error converting configuration: %v\n", err)
		return 1
	}

	contextMap, totalForwards, err := converter.GetConversionSummary(input)
	if err != nil {
		fprintf(stderr, "Warning: Could not generate summary: %v\n", err)
		return 0
	}
	fprintf(stdout, "Successfully converted %d forwards from %s to %s\n", totalForwards, input, output)
	fprintf(stdout, "Generated configuration with:\n")
	for ctx, namespaces := range contextMap {
		fprintf(stdout, "  - Context '%s':\n", ctx)
		for ns, count := range namespaces {
			fprintf(stdout, "    - Namespace '%s': %d forwards\n", ns, count)
		}
	}
	return 0
}

// runHeadless runs the daemon-style mode: no UI, signal-driven SIGHUP reloads,
// graceful shutdown on ctx.Done() (which is cancelled by SIGINT/SIGTERM).
func runHeadless(ctx context.Context, opts runOptions, cfg *config.Config, deps *runtimeDeps, stderr io.Writer) int {
	if startErr := deps.manager.Start(cfg); startErr != nil {
		fprintf(stderr, "Error starting forwards: %v\n", startErr)
		return 1
	}

	// SIGHUP triggers reload only — separate from the ctx-driven shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	defer signal.Stop(sigChan)

	watcher, watcherErr := config.NewWatcher(opts.configFile, func(newCfg *config.Config) error {
		return deps.manager.Reload(newCfg)
	}, opts.verbose, config.WithContextSelection(opts.contexts))
	watcherStarted := false
	if watcherErr != nil {
		if opts.verbose {
			log.Printf("Warning: Failed to setup config watcher: %v", watcherErr)
			log.Printf("Hot-reload will not be available")
		}
	} else {
		watcher.Start()
		watcherStarted = true
	}
	defer func() {
		if watcherStarted {
			watcher.Stop()
		}
	}()

	if opts.verbose {
		log.Printf("Headless mode started. Press Ctrl+C to stop")
	}

	for {
		select {
		case <-ctx.Done():
			return shutdownManager(ctx, deps.manager, opts.verbose)
		case <-sigChan:
			if opts.verbose {
				log.Printf("Received SIGHUP, reloading configuration...")
			}
			reloadFromSignal(opts, deps.manager, opts.verbose, nil)
		}
	}
}

// runVerboseTable runs the simple table UI with periodic redraws and SIGHUP
// reload, exiting cleanly when ctx is cancelled.
func runVerboseTable(ctx context.Context, opts runOptions, cfg *config.Config, deps *runtimeDeps, stderr io.Writer) int {
	tableUI := ui.NewTableUI(opts.verbose, cfg)
	deps.manager.SetStatusUI(tableUI)

	// Background update check (best effort).
	go func() {
		checker := version.NewChecker(githubOwner, githubRepo, appVersion)
		uctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if update := checker.CheckForUpdate(uctx); update != nil {
			log.Printf("Update available: v%s (current: v%s) - %s",
				update.LatestVersion, update.CurrentVersion, update.ReleaseURL)
		}
	}()

	if startErr := deps.manager.Start(cfg); startErr != nil {
		fprintf(stderr, "Error starting forwards: %v\n", startErr)
		return 1
	}

	tableUI.RenderInitial()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	defer signal.Stop(sigChan)

	// Periodic table refresh — driven by ctx.Done() so it exits cleanly.
	tickerDone := make(chan struct{})
	go func() {
		defer close(tickerDone)
		ticker := time.NewTicker(tableUpdateInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tableUI.Render()
			}
		}
	}()

	watcher, watchErr := config.NewWatcher(opts.configFile, func(newCfg *config.Config) error {
		if err := deps.manager.Reload(newCfg); err != nil {
			return err
		}
		tableUI.SetColumns(newCfg)
		return nil
	}, opts.verbose, config.WithContextSelection(opts.contexts))
	watcherActive := false
	if watchErr != nil {
		log.Printf("Warning: Failed to setup config watcher: %v", watchErr)
		log.Printf("Hot-reload will not be available")
	} else {
		watcher.Start()
		watcherActive = true
	}
	defer func() {
		if watcherActive {
			watcher.Stop()
		}
	}()

	log.Printf("Press Ctrl+C to stop")

	for {
		select {
		case <-ctx.Done():
			<-tickerDone
			return shutdownManager(ctx, deps.manager, opts.verbose)
		case <-sigChan:
			log.Printf("Received SIGHUP, reloading configuration...")
			reloadFromSignal(opts, deps.manager, true, tableUI.SetColumns)
		}
	}
}

// runInteractive runs the bubbletea TUI. Cannot be exercised in non-TTY tests.
func runInteractive(ctx context.Context, opts runOptions, cfg *config.Config, deps *runtimeDeps, stderr io.Writer) int {
	bubbleTeaUI := ui.NewBubbleTeaUI(func(id string, enable bool) {
		if enable {
			_ = deps.manager.EnableForward(id)
		} else {
			_ = deps.manager.DisableForward(id)
		}
	}, appVersion)
	bubbleTeaUI.SetColumns(cfg)
	bubbleTeaUI.SetWizardDependencies(deps.discovery, deps.mutator, opts.configFile)
	bubbleTeaUI.SetActiveContexts(opts.contexts)
	bubbleTeaUI.SetHTTPLogSubscriber(makeHTTPLogSubscriber(deps.manager))

	go func() {
		checker := version.NewChecker(githubOwner, githubRepo, appVersion)
		uctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if update := checker.CheckForUpdate(uctx); update != nil {
			bubbleTeaUI.SetUpdateAvailable(update.LatestVersion, update.ReleaseURL)
		}
	}()

	deps.manager.SetStatusUI(bubbleTeaUI)

	if startErr := deps.manager.Start(cfg); startErr != nil {
		fprintf(stderr, "Error starting forwards: %v\n", startErr)
		return 1
	}

	var watcher *config.Watcher
	watcher, err := config.NewWatcher(opts.configFile, func(newCfg *config.Config) error {
		if err := deps.manager.Reload(newCfg); err != nil {
			return err
		}
		bubbleTeaUI.SetColumns(newCfg)
		return nil
	}, opts.verbose, config.WithContextSelection(opts.contexts))
	if err == nil {
		watcher.Start()
	}

	cleanup := func() {
		bubbleTeaUI.Stop()
		deps.manager.Stop()
		if watcher != nil {
			watcher.Stop()
		}
	}

	// Wire ctx cancellation to UI shutdown so SIGINT/SIGTERM exit cleanly.
	stopWatcher := make(chan struct{})
	defer close(stopWatcher)
	go func() {
		select {
		case <-ctx.Done():
			cleanup()
		case <-stopWatcher:
		}
	}()

	time.Sleep(initialForwardSettleTime)

	if startErr := bubbleTeaUI.Start(); startErr != nil {
		fprintf(stderr, "Failed to start UI: %v\n", startErr)
		cleanup()
		return 1
	}
	cleanup()
	return 0
}

// makeHTTPLogSubscriber builds the subscriber callback used by the bubbletea UI.
func makeHTTPLogSubscriber(manager *forward.Manager) ui.HTTPLogSubscriber {
	return func(forwardID string, callback func(entry ui.HTTPLogEntry)) func() {
		worker := manager.GetWorker(forwardID)
		if worker == nil {
			logger.Debug("HTTP log subscription failed: worker not found", map[string]any{
				"forward_id": forwardID,
			})
			return func() {}
		}
		proxy := worker.GetHTTPProxy()
		if proxy == nil {
			logger.Debug("HTTP log subscription skipped: proxy not enabled", map[string]any{
				"forward_id": forwardID,
			})
			return func() {}
		}
		proxyLogger := proxy.GetLogger()
		if proxyLogger == nil {
			logger.Debug("HTTP log subscription failed: logger not available", map[string]any{
				"forward_id": forwardID,
			})
			return func() {}
		}

		proxyLogger.AddCallback(func(entry httplog.Entry) {
			uiEntry := ui.HTTPLogEntry{
				RequestID:  entry.RequestID,
				Timestamp:  entry.Timestamp.Format("15:04:05"),
				Direction:  entry.Direction,
				Method:     entry.Method,
				Path:       entry.Path,
				StatusCode: entry.StatusCode,
				LatencyMs:  entry.LatencyMs,
				BodySize:   entry.BodySize,
				Error:      entry.Error,
			}
			switch entry.Direction {
			case "request":
				uiEntry.RequestHeaders = entry.Headers
				uiEntry.RequestBody = entry.Body
			case "response":
				uiEntry.ResponseHeaders = entry.Headers
				uiEntry.ResponseBody = entry.Body
			}
			callback(uiEntry)
		})

		return func() {
			proxyLogger.ClearCallbacks()
		}
	}
}

// shutdownManager stops the forward manager with a 5s timeout, returning 0 on
// success or after timeout (we always exit cleanly from a shutdown signal).
func shutdownManager(ctx context.Context, manager *forward.Manager, verbose bool) int {
	if verbose {
		log.Printf("Received shutdown signal, stopping...")
	}
	shutdownDone := make(chan struct{})
	go func() {
		manager.Stop()
		close(shutdownDone)
	}()
	select {
	case <-shutdownDone:
		if verbose {
			log.Printf("Graceful shutdown complete")
		}
	case <-time.After(5 * time.Second):
		if verbose {
			log.Printf("Shutdown timed out, forcing exit...")
		}
	}
	_ = ctx // ctx may already be done; we still wait up to 5s for graceful stop.
	return 0
}
