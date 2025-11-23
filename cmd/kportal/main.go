package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/converter"
	"github.com/nvm/kportal/internal/forward"
	"github.com/nvm/kportal/internal/k8s"
	"github.com/nvm/kportal/internal/ui"
	"k8s.io/klog/v2"
)

const (
	defaultConfigFile = ".kportal.yaml"
)

var (
	configFile    = flag.String("c", defaultConfigFile, "Path to configuration file")
	verbose       = flag.Bool("v", false, "Enable verbose logging")
	check         = flag.Bool("check", false, "Validate configuration and exit")
	showVersion   = flag.Bool("version", false, "Show version and exit")
	convertInput  = flag.String("convert", "", "Convert kftray JSON config to kportal YAML (provide input file path)")
	convertOutput = flag.String("convert-output", ".kportal.yaml", "Output file for converted configuration")
	version       = "0.1.0" // Set via ldflags during build
)

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("kportal version %s\n", version)
		os.Exit(0)
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

		// Disable klog (used by kubernetes client-go)
		// This prevents kubernetes portforward errors from appearing in the terminal
		klog.SetOutput(io.Discard)
		klog.LogToStderr(false)
		// Set to high verbosity level to suppress all levels
		var klogFlags flag.FlagSet
		klogFlags.Set("logtostderr", "false")
		klogFlags.Set("alsologtostderr", "false")
		klogFlags.Set("stderrthreshold", "FATAL")
		klogFlags.Set("v", "0")
	} else {
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
		log.Printf("kportal v%s", version)
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
	manager := forward.NewManager(*verbose)

	// Create UI (bubbletea for interactive, simple table for verbose)
	var bubbleTeaUI *ui.BubbleTeaUI
	var tableUI *ui.TableUI

	if !*verbose {
		// Interactive mode with bubbletea
		bubbleTeaUI = ui.NewBubbleTeaUI(func(id string, enable bool) {
			if enable {
				manager.EnableForward(id)
			} else {
				manager.DisableForward(id)
			}
		}, version)

		// Set wizard dependencies
		// Note: mutator is always available (for delete/edit), discovery requires valid kubeconfig (for add)
		bubbleTeaUI.SetWizardDependencies(discovery, mutator, *configFile)

		manager.SetStatusUI(bubbleTeaUI)
	} else {
		// Verbose mode with simple table
		tableUI = ui.NewTableUI(*verbose)
		manager.SetStatusUI(tableUI)
	}

	// Start forwards
	if err := manager.Start(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting forwards: %v\n", err)
		os.Exit(1)
	}

	if *verbose {
		// Verbose mode - use simple table with periodic updates
		tableUI.RenderInitial()

		// Setup signal handling
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

		// Start table update loop
		go func() {
			ticker := time.NewTicker(2 * time.Second)
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
				manager.Stop()
				os.Exit(0)
			}
		}
	} else {
		// Interactive mode with bubbletea
		// Setup config watcher in background
		watcher, err := config.NewWatcher(*configFile, func(newCfg *config.Config) error {
			return manager.Reload(newCfg)
		}, *verbose)
		if err == nil {
			watcher.Start()
			defer watcher.Stop()
		}

		// Setup signal handler for clean shutdown
		go func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
			<-sigChan
			bubbleTeaUI.Stop()
			manager.Stop()
			os.Exit(0)
		}()

		// Give a moment for initial forwards to be added
		time.Sleep(100 * time.Millisecond)

		// Start the bubbletea app (blocks until quit)
		if err := bubbleTeaUI.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start UI: %v\n", err)
			manager.Stop()
			os.Exit(1)
		}

		// Clean shutdown
		manager.Stop()
	}
}
