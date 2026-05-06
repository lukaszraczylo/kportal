package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/lukaszraczylo/kportal/internal/k8s"
	"github.com/lukaszraczylo/kportal/internal/logger"
	"github.com/lukaszraczylo/kportal/internal/ui"
)

// runGenerate parses generate-specific flags, validates them, and runs the
// generate flow. Returns the process exit code.
func runGenerate(args []string) int {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: kportal generate --context=NAME [--config=PATH] [--dry-run]\n\n")
		fmt.Fprintf(os.Stderr, "Discover services in the chosen Kubernetes context, pick which ones\n")
		fmt.Fprintf(os.Stderr, "to forward, and append them to the kportal config file.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
	}
	contextFlag := fs.String("context", "", "Kubernetes context to scan (required)")
	configFlag := fs.String("config", defaultConfigFile, "Path to kportal configuration file")
	dryRunFlag := fs.Bool("dry-run", false, "Print the planned forwards but do not modify the config")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if *contextFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: --context is required")
		fs.Usage()
		return 1
	}

	// Initialise a discard logger so kubernetes client-go silence is honoured —
	// the bubbletea TUI cannot tolerate stderr writes.
	logger.Init(logger.LevelError, logger.FormatText, io.Discard)

	// Resolve and sanitise config path the same way main does.
	configPath, ok := resolveGenerateConfigPath(*configFlag)
	if !ok {
		return 1
	}

	// Build kubernetes client pool and verify the requested context exists.
	pool, err := k8s.NewClientPool()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load kubeconfig: %v\n", err)
		return 1
	}
	contexts, err := pool.ListContexts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to list kubeconfig contexts: %v\n", err)
		return 1
	}
	if !contains(contexts, *contextFlag) {
		fmt.Fprintf(os.Stderr, "Error: context %q not found in kubeconfig\n", *contextFlag)
		fmt.Fprintf(os.Stderr, "Available contexts: %s\n", strings.Join(contexts, ", "))
		return 1
	}
	discovery := k8s.NewDiscovery(pool)
	mutator := config.NewMutator(configPath)

	// Load existing config (or treat as empty if missing) to gather already-configured forwards.
	var existingForwards []config.Forward
	cfg, loadErr := config.LoadConfig(configPath)
	switch {
	case loadErr == nil:
		existingForwards = cfg.GetAllForwards()
	case errors.Is(loadErr, config.ErrConfigNotFound):
		// Config does not exist yet — that's fine; we'll create it on save.
		existingForwards = nil
	default:
		fmt.Fprintf(os.Stderr, "Error: failed to load config: %v\n", loadErr)
		return 1
	}

	result, err := ui.RunGenerate(discovery, mutator, *contextFlag, configPath, *dryRunFlag, existingForwards)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if result.Cancelled {
		fmt.Fprintln(os.Stderr, "Cancelled.")
		return 1
	}

	if result.UsedDryRun {
		fmt.Printf("[dry-run] Would add %d forwards to %s\n", len(result.PlannedForwards), configPath)
		for _, f := range result.PlannedForwards {
			fmt.Printf("  %d → %s/%s/%s:%d\n", f.LocalPort, f.GetContext(), f.GetNamespace(), f.Resource, f.Port)
		}
		if result.SkippedNonTCP > 0 {
			fmt.Fprintf(os.Stderr, "Warning: skipped %d non-TCP service ports (kportal forward layer is TCP-only)\n", result.SkippedNonTCP)
		}
		return 0
	}

	if len(result.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "Added %d forwards before error; remaining failed:\n", result.Added)
		for _, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		return 1
	}

	fmt.Printf("Added %d forwards to %s\n", result.Added, configPath)
	if result.SkippedNonTCP > 0 {
		fmt.Fprintf(os.Stderr, "Warning: skipped %d non-TCP service ports (kportal forward layer is TCP-only)\n", result.SkippedNonTCP)
	}
	return 0
}

// resolveGenerateConfigPath mirrors the path validation main applies before
// loading config: absolute, cleaned, and not inside protected system directories.
func resolveGenerateConfigPath(path string) (string, bool) {
	if path == "" {
		fmt.Fprintln(os.Stderr, "Error: --config cannot be empty")
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid config path: %v\n", err)
		return "", false
	}
	abs = filepath.Clean(abs)
	for _, sysDir := range []string{"/etc", "/sys", "/proc", "/dev"} {
		if strings.HasPrefix(abs, sysDir) {
			fmt.Fprintf(os.Stderr, "Error: config file cannot be in system directory: %s\n", sysDir)
			return "", false
		}
	}
	return abs, true
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
