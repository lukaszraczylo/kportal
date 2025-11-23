package config

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// ReloadCallback is called when the configuration file changes.
// It receives the new configuration and should return an error if the reload fails.
type ReloadCallback func(*Config) error

// Watcher watches a configuration file for changes and triggers hot-reload.
type Watcher struct {
	configPath string
	callback   ReloadCallback
	watcher    *fsnotify.Watcher
	done       chan struct{}
	verbose    bool
}

// NewWatcher creates a new file watcher for the given config file.
func NewWatcher(configPath string, callback ReloadCallback, verbose bool) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Watch the directory instead of the file to handle atomic writes
	// (many editors delete and recreate files on save)
	dir := filepath.Dir(absPath)
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch directory %s: %w", dir, err)
	}

	return &Watcher{
		configPath: absPath,
		callback:   callback,
		watcher:    watcher,
		done:       make(chan struct{}),
		verbose:    verbose,
	}, nil
}

// Start begins watching the configuration file for changes.
func (w *Watcher) Start() {
	go w.watch()
}

// Stop stops watching the configuration file.
func (w *Watcher) Stop() {
	close(w.done)
	w.watcher.Close()
}

// watch runs the file watching loop.
func (w *Watcher) watch() {
	if w.verbose {
		log.Printf("Watching configuration file: %s", w.configPath)
	}

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Only process events for our config file
			eventPath, err := filepath.Abs(event.Name)
			if err != nil {
				if w.verbose {
					log.Printf("Failed to resolve event path: %v", err)
				}
				continue
			}

			if eventPath != w.configPath {
				continue
			}

			// Handle write and create events (create happens on atomic writes)
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				if w.verbose {
					log.Printf("Configuration file changed, reloading...")
				}
				w.handleReload()
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("File watcher error: %v", err)

		case <-w.done:
			return
		}
	}
}

// handleReload loads and validates the new configuration, then calls the callback.
func (w *Watcher) handleReload() {
	// Load new configuration
	newCfg, err := LoadConfig(w.configPath)
	if err != nil {
		log.Printf("Failed to load configuration: %v", err)
		log.Printf("Keeping previous configuration active")
		return
	}

	// Validate new configuration
	validator := NewValidator()
	if errs := validator.ValidateConfig(newCfg); len(errs) > 0 {
		log.Printf("Configuration validation failed:")
		log.Print(FormatValidationErrors(errs))
		log.Printf("Keeping previous configuration active")
		return
	}

	// Call reload callback
	if err := w.callback(newCfg); err != nil {
		log.Printf("Failed to apply new configuration: %v", err)
		log.Printf("Keeping previous configuration active")
		return
	}

	if w.verbose {
		log.Printf("Configuration reloaded successfully")
	}
}
