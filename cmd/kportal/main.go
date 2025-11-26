package main

import (
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
	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/converter"
	"github.com/nvm/kportal/internal/forward"
	"github.com/nvm/kportal/internal/httplog"
	"github.com/nvm/kportal/internal/k8s"
	"github.com/nvm/kportal/internal/logger"
	"github.com/nvm/kportal/internal/mdns"
	"github.com/nvm/kportal/internal/ui"
	"github.com/nvm/kportal/internal/version"
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

var (
	configFile    = flag.String("c", defaultConfigFile, "Path to configuration file")
	verbose       = flag.Bool("v", false, "Enable verbose logging")
	headless      = flag.Bool("headless", false, "Run in headless mode (no UI, for background/daemon use)")
	logFormat     = flag.String("log-format", "text", "Log format: text or json")
	check         = flag.Bool("check", false, "Validate configuration and exit")
	showVersion   = flag.Bool("version", false, "Show version and exit")
	checkUpdate   = flag.Bool("update", false, "Check for updates and exit")
	convertInput  = flag.String("convert", "", "Convert kftray JSON config to kportal YAML (provide input file path)")
	convertOutput = flag.String("convert-output", ".kportal.yaml", "Output file for converted configuration")
	appVersion    = "0.1.0" // Set via ldflags during build
)

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("kportal version %s\n", appVersion)
		os.Exit(0)
	}

	if *checkUpdate {
		checkForUpdates()
		os.Exit(0)
	}

	// Validate config path security
	if *configFile != "" {
		absConfigPath, err := filepath.Abs(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid config path: %v\n", err)
			os.Exit(1)
		}
		absConfigPath = filepath.Clean(absConfigPath)

		// Block system directories
		systemDirs := []string{"/etc", "/sys", "/proc", "/dev"}
		for _, sysDir := range systemDirs {
			if strings.HasPrefix(absConfigPath, sysDir) {
				fmt.Fprintf(os.Stderr, "Error: Config file cannot be in system directory: %s\n", sysDir)
				os.Exit(1)
			}
		}

		*configFile = absConfigPath
	}

	// Initialize structured logger
	var logLevel logger.Level
	var logFmt logger.Format
	var logOutput io.Writer

	if *verbose {
		logLevel = logger.LevelDebug
		logOutput = os.Stderr
	} else {
		logLevel = logger.LevelInfo
		logOutput = io.Discard // Silence logger in non-verbose/headless mode to prevent UI corruption
	}

	switch *logFormat {
	case "json":
		logFmt = logger.FormatJSON
	default:
		logFmt = logger.FormatText
	}

	logger.Init(logLevel, logFmt, logOutput)

	// Configure klog (used by kubernetes client-go) to route through our logger
	// This prevents k8s logs from interfering with the UI
	//
	// klog v2 uses multiple output mechanisms:
	// 1. SetOutput() - for basic text output
	// 2. SetLogger() - for structured/error logs (logr interface)
	//
	// We must configure BOTH to capture all logs including error messages
	// that would otherwise bypass SetOutput() and write directly to stderr.
	klog.LogToStderr(false) // Disable direct stderr writes
	if *verbose {
		// In verbose mode, route all klog through our structured logger at DEBUG level
		klogLogger := logger.New(logger.LevelDebug, logFmt, os.Stderr)

		// Configure text output routing
		klogWriter := logger.NewKlogWriter(klogLogger)
		klog.SetOutput(klogWriter)

		// Configure structured/error log routing via logr interface
		// This captures "Unhandled Error" and other structured logs that bypass SetOutput
		logrSink := logger.NewLogrAdapter(klogLogger)
		klog.SetLogger(logr.New(logrSink))
	} else {
		// In non-verbose mode, completely silence ALL klog output
		klog.SetOutput(io.Discard)

		// Also silence structured/error logs via a discard logger
		silentLogger := logger.New(logger.LevelError+1, logFmt, io.Discard) // Level above ERROR = silence all
		logrSink := logger.NewLogrAdapter(silentLogger)
		klog.SetLogger(logr.New(logrSink))
	}

	// Handle conversion mode
	if *convertInput != "" {
		if err := converter.ConvertKFTrayToKPortal(*convertInput, *convertOutput); err != nil {
			fmt.Fprintf(os.Stderr, "Error converting configuration: %v\n", err)
			os.Exit(1)
		}

		// Print summary
		contextMap, totalForwards, err := converter.GetConversionSummary(*convertInput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not generate summary: %v\n", err)
		} else {
			fmt.Printf("Successfully converted %d forwards from %s to %s\n", totalForwards, *convertInput, *convertOutput)
			fmt.Printf("Generated configuration with:\n")
			for ctx, namespaces := range contextMap {
				fmt.Printf("  - Context '%s':\n", ctx)
				for ns, count := range namespaces {
					fmt.Printf("    - Namespace '%s': %d forwards\n", ns, count)
				}
			}
		}
		os.Exit(0)
	}

	if !*verbose {
		// In interactive mode, disable ALL logging to avoid interfering with bubbletea UI
		log.SetOutput(io.Discard)
		log.SetPrefix("")
		log.SetFlags(0)
	} else {
		// Verbose mode - enable standard log formatting
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Validate configuration
	validator := config.NewValidator()
	if errs := validator.ValidateConfig(cfg); len(errs) > 0 {
		fmt.Fprint(os.Stderr, config.FormatValidationErrors(errs))
		os.Exit(1)
	}

	if *check {
		fmt.Println("Configuration is valid")
		os.Exit(0)
	}

	// Only log startup messages in verbose mode
	if *verbose {
		log.Printf("kportal v%s", appVersion)
		log.Printf("Loading configuration from: %s", *configFile)
	}

	// Create Kubernetes client pool and discovery for wizards
	pool, err := k8s.NewClientPool()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to create k8s client pool: %v\n", err)
		fmt.Fprintf(os.Stderr, "Add/remove wizards will not be available\n")
	}
	discovery := k8s.NewDiscovery(pool)
	mutator := config.NewMutator(*configFile)

	// Create forward manager
	manager, err := forward.NewManager(*verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating forward manager: %v\n", err)
		os.Exit(1)
	}

	// Create mDNS publisher if enabled in config
	mdnsPublisher := mdns.NewPublisher(cfg.IsMDNSEnabled())
	manager.SetMDNSPublisher(mdnsPublisher)

	if cfg.IsMDNSEnabled() && *verbose {
		log.Printf("mDNS hostname publishing enabled - aliases will be accessible via <alias>.local")
	}

	// Create UI based on mode:
	// - headless: no UI at all (background daemon)
	// - verbose: simple table UI with logging
	// - default: interactive bubbletea TUI
	var bubbleTeaUI *ui.BubbleTeaUI
	var tableUI *ui.TableUI

	if *headless {
		// Headless mode - no UI, just run forwards in background
		// StatusUI remains nil, manager will handle this gracefully
		if *verbose {
			log.Printf("Running in headless mode with verbose logging")
		}
	} else if *verbose {
		// Verbose mode with simple table
		tableUI = ui.NewTableUI(*verbose)
		manager.SetStatusUI(tableUI)

		// Check for updates and print to log
		go func() {
			checker := version.NewChecker(githubOwner, githubRepo, appVersion)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if update := checker.CheckForUpdate(ctx); update != nil {
				log.Printf("Update available: v%s (current: v%s) - %s",
					update.LatestVersion, update.CurrentVersion, update.ReleaseURL)
			}
		}()
	} else {
		// Interactive mode with bubbletea
		bubbleTeaUI = ui.NewBubbleTeaUI(func(id string, enable bool) {
			if enable {
				manager.EnableForward(id)
			} else {
				manager.DisableForward(id)
			}
		}, appVersion)

		// Set wizard dependencies
		// Note: mutator is always available (for delete/edit), discovery requires valid kubeconfig (for add)
		bubbleTeaUI.SetWizardDependencies(discovery, mutator, *configFile)

		// Set HTTP log subscriber to enable live log viewing
		bubbleTeaUI.SetHTTPLogSubscriber(func(forwardID string, callback func(entry ui.HTTPLogEntry)) func() {
			worker := manager.GetWorker(forwardID)
			if worker == nil {
				return func() {} // No-op cleanup
			}

			proxy := worker.GetHTTPProxy()
			if proxy == nil {
				return func() {} // HTTP logging not enabled for this forward
			}

			proxyLogger := proxy.GetLogger()
			if proxyLogger == nil {
				return func() {}
			}

			// Subscribe to log entries
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

				// Populate headers based on direction
				if entry.Direction == "request" {
					uiEntry.RequestHeaders = entry.Headers
					uiEntry.RequestBody = entry.Body
				} else if entry.Direction == "response" {
					uiEntry.ResponseHeaders = entry.Headers
					uiEntry.ResponseBody = entry.Body
				}

				callback(uiEntry)
			})

			// Return cleanup function
			return func() {
				proxyLogger.ClearCallbacks()
			}
		})

		// Check for updates in background (non-blocking)
		go func() {
			checker := version.NewChecker(githubOwner, githubRepo, appVersion)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if update := checker.CheckForUpdate(ctx); update != nil {
				bubbleTeaUI.SetUpdateAvailable(update.LatestVersion, update.ReleaseURL)
			}
		}()

		manager.SetStatusUI(bubbleTeaUI)
	}

	// Start forwards
	if err := manager.Start(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting forwards: %v\n", err)
		os.Exit(1)
	}

	if *headless {
		// Headless mode - no UI, run as background daemon
		// Setup signal handling
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

		// Setup config watcher for hot-reload
		watcher, err := config.NewWatcher(*configFile, func(newCfg *config.Config) error {
			return manager.Reload(newCfg)
		}, *verbose)
		if err != nil {
			if *verbose {
				log.Printf("Warning: Failed to setup config watcher: %v", err)
				log.Printf("Hot-reload will not be available")
			}
		} else {
			watcher.Start()
			defer watcher.Stop()
		}

		if *verbose {
			log.Printf("Headless mode started. Press Ctrl+C to stop")
		}

		// Wait for signals
		for {
			sig := <-sigChan
			switch sig {
			case syscall.SIGHUP:
				if *verbose {
					log.Printf("Received SIGHUP, reloading configuration...")
				}
				newCfg, err := config.LoadConfig(*configFile)
				if err != nil {
					if *verbose {
						log.Printf("Failed to reload config: %v", err)
					}
					continue
				}

				if errs := validator.ValidateConfig(newCfg); len(errs) > 0 {
					if *verbose {
						log.Printf("Config validation failed:")
						log.Print(config.FormatValidationErrors(errs))
					}
					continue
				}

				if err := manager.Reload(newCfg); err != nil {
					if *verbose {
						log.Printf("Failed to reload: %v", err)
					}
				}

			case os.Interrupt, syscall.SIGTERM:
				if *verbose {
					log.Printf("Received shutdown signal, stopping...")
				}

				// Graceful shutdown with timeout
				shutdownDone := make(chan struct{})
				go func() {
					manager.Stop()
					close(shutdownDone)
				}()

				select {
				case <-shutdownDone:
					if *verbose {
						log.Printf("Graceful shutdown complete")
					}
				case <-time.After(5 * time.Second):
					if *verbose {
						log.Printf("Shutdown timed out, forcing exit...")
					}
				case sig := <-sigChan:
					if *verbose {
						log.Printf("Received second signal (%v), forcing exit...", sig)
					}
				}
				os.Exit(0)
			}
		}
	} else if *verbose {
		// Verbose mode - use simple table with periodic updates
		tableUI.RenderInitial()

		// Setup signal handling
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

		// Start table update loop
		go func() {
			ticker := time.NewTicker(tableUpdateInterval)
			defer ticker.Stop()
			for range ticker.C {
				tableUI.Render()
			}
		}()

		// Setup config watcher for hot-reload
		watcher, err := config.NewWatcher(*configFile, func(newCfg *config.Config) error {
			return manager.Reload(newCfg)
		}, *verbose)
		if err != nil {
			log.Printf("Warning: Failed to setup config watcher: %v", err)
			log.Printf("Hot-reload will not be available")
		} else {
			watcher.Start()
			defer watcher.Stop()
		}

		log.Printf("Press Ctrl+C to stop")

		// Wait for signals
		for {
			sig := <-sigChan
			switch sig {
			case syscall.SIGHUP:
				log.Printf("Received SIGHUP, reloading configuration...")
				newCfg, err := config.LoadConfig(*configFile)
				if err != nil {
					log.Printf("Failed to reload config: %v", err)
					continue
				}

				if errs := validator.ValidateConfig(newCfg); len(errs) > 0 {
					log.Printf("Config validation failed:")
					log.Print(config.FormatValidationErrors(errs))
					continue
				}

				if err := manager.Reload(newCfg); err != nil {
					log.Printf("Failed to reload: %v", err)
				}

			case os.Interrupt, syscall.SIGTERM:
				log.Printf("Received shutdown signal, stopping...")

				// Graceful shutdown with timeout - force exit if it takes too long
				shutdownDone := make(chan struct{})
				go func() {
					manager.Stop()
					close(shutdownDone)
				}()

				select {
				case <-shutdownDone:
					log.Printf("Graceful shutdown complete")
				case <-time.After(5 * time.Second):
					log.Printf("Shutdown timed out, forcing exit...")
				case sig := <-sigChan:
					// Second signal received - force exit immediately
					log.Printf("Received second signal (%v), forcing exit...", sig)
				}
				os.Exit(0)
			}
		}
	} else {
		// Interactive mode with bubbletea
		// Setup config watcher in background
		var watcher *config.Watcher
		watcher, err = config.NewWatcher(*configFile, func(newCfg *config.Config) error {
			return manager.Reload(newCfg)
		}, *verbose)
		if err == nil {
			watcher.Start()
		}

		// Cleanup function to ensure all resources are released
		cleanup := func() {
			bubbleTeaUI.Stop()
			manager.Stop()
			if watcher != nil {
				watcher.Stop()
			}
		}

		// Setup signal handler for clean shutdown
		go func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
			<-sigChan
			cleanup()
			os.Exit(0)
		}()

		// Give a moment for initial forwards to be added
		time.Sleep(initialForwardSettleTime)

		// Start the bubbletea app (blocks until quit)
		if err := bubbleTeaUI.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start UI: %v\n", err)
			cleanup()
			os.Exit(1)
		}

		// Clean shutdown (normal exit via UI quit)
		cleanup()
	}
}

// checkForUpdates checks for available updates and prints the result
func checkForUpdates() {
	fmt.Printf("kportal version %s\n", appVersion)
	fmt.Println("Checking for updates...")

	checker := version.NewChecker(githubOwner, githubRepo, appVersion)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	update := checker.CheckForUpdate(ctx)
	if update == nil {
		fmt.Println("You are running the latest version.")
		return
	}

	fmt.Printf("\nUpdate available: v%s\n", update.LatestVersion)
	fmt.Printf("Download: %s\n", update.ReleaseURL)
	fmt.Println("\nTo update, download the latest release from the URL above")
	fmt.Println("or use your package manager (e.g., 'brew upgrade kportal').")
}
