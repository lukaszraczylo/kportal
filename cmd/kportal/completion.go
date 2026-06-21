package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/lukaszraczylo/kportal/internal/complete"
)

// completionCmd handles shell completion generation and installation
func completionCmd(args []string) int {
	fs := flag.NewFlagSet("completion", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		installFlag bool
		shellFlag   string
		uninstall   bool
	)

	fs.BoolVar(&installFlag, "install", false, "Install completions for the shell")
	fs.BoolVar(&uninstall, "uninstall", false, "Uninstall completions")
	fs.StringVar(&shellFlag, "shell", "", "Shell type: bash, zsh, or fish (auto-detected if empty)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			printCompletionHelp()
			return 0
		}
		return 2
	}

	// Determine shell type
	var shell complete.Shell
	if shellFlag != "" {
		switch shellFlag {
		case "bash":
			shell = complete.ShellBash
		case "zsh":
			shell = complete.ShellZsh
		case "fish":
			shell = complete.ShellFish
		default:
			fprintf(os.Stderr, "Error: unknown shell %q (use bash, zsh, or fish)\n", shellFlag)
			return 1
		}
	} else {
		shell = complete.AutoDetectShell()
	}

	// Handle uninstall
	if uninstall {
		installer := complete.NewInstaller(shell)
		if err := installer.Uninstall(); err != nil {
			fprintf(os.Stderr, "Error uninstalling completions: %v\n", err)
			return 1
		}
		fmt.Println("✅ Completions uninstalled")
		return 0
	}

	// Handle install
	if installFlag {
		if err := complete.InstallCompletions(shell); err != nil {
			fprintf(os.Stderr, "Error installing completions: %v\n", err)
			return 1
		}
		return 0
	}

	// Print completion script to stdout
	if err := complete.Print(shell); err != nil {
		fprintf(os.Stderr, "Error generating completions: %v\n", err)
		return 1
	}

	return 0
}

func printCompletionHelp() {
	fprintf(os.Stdout, `Generate shell completions for kportal.

Usage:
  kportal completion [flags]

Flags:
  --install        Install completions for the current shell
  --uninstall      Remove installed completions
  --shell <type>   Shell type: bash, zsh, or fish (auto-detected)

Examples:
  # Generate and source completions (bash)
  source <(kportal completion)

  # Install completions (requires shell restart)
  kportal completion --install

  # Install for specific shell
  kportal completion --install --shell zsh

  # Uninstall completions
  kportal completion --uninstall

Shell-specific setup:

  Bash (~/.bashrc):
    source <(kportal completion)

  Zsh (~/.zshrc):
    autoload -Uz compinit && compinit
    source <(kportal completion)

  Fish (~/.config/fish/config.fish):
    kportal completion --install --shell fish
    # Or manually:
    kportal completion --shell fish > ~/.config/fish/completions/kportal.fish
`)
}
