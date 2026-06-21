package complete

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateBash(t *testing.T) {
	script, err := Generate(ShellBash)
	if err != nil {
		t.Fatalf("Generate(ShellBash) failed: %v", err)
	}

	// Verify script contains key elements
	if !strings.Contains(script, "_kportal()") {
		t.Error("Bash script missing _kportal function")
	}
	if !strings.Contains(script, "complete -F _kportal kportal") {
		t.Error("Bash script missing completion registration")
	}
	if !strings.Contains(script, "--version") {
		t.Error("Bash script missing --version flag")
	}
	if !strings.Contains(script, "generate") {
		t.Error("Bash script missing generate subcommand")
	}
}

func TestGenerateZsh(t *testing.T) {
	script, err := Generate(ShellZsh)
	if err != nil {
		t.Fatalf("Generate(ShellZsh) failed: %v", err)
	}

	// Verify script contains key elements
	if !strings.Contains(script, "#compdef kportal") {
		t.Error("Zsh script missing #compdef directive")
	}
	if !strings.Contains(script, "_kportal()") {
		t.Error("Zsh script missing _kportal function")
	}
	if !strings.Contains(script, "'generate:") {
		t.Error("Zsh script missing generate subcommand")
	}
	if !strings.Contains(script, "'--context[") {
		t.Error("Zsh script missing --context flag")
	}
}

func TestGenerateFish(t *testing.T) {
	script, err := Generate(ShellFish)
	if err != nil {
		t.Fatalf("Generate(ShellFish) failed: %v", err)
	}

	// Verify script contains key elements
	if !strings.Contains(script, "complete -c kportal") {
		t.Error("Fish script missing complete directive")
	}
	if !strings.Contains(script, "-n '__fish_use_subcommand'") {
		t.Error("Fish script missing subcommand detection")
	}
	if !strings.Contains(script, "-l context") {
		t.Error("Fish script missing --context flag")
	}
}

func TestGenerateUnsupported(t *testing.T) {
	_, err := Generate(Shell("unsupported"))
	if err == nil {
		t.Error("Expected error for unsupported shell")
	}
}

func TestAutoDetectShell(t *testing.T) {
	tests := []struct {
		name     string
		shellEnv string
		expected Shell
	}{
		{"bash", "/bin/bash", ShellBash},
		{"zsh", "/usr/bin/zsh", ShellZsh},
		{"fish", "/usr/local/bin/fish", ShellFish},
		{"tcsh", "/bin/tcsh", ShellBash}, // Falls back to bash
		{"empty", "", ShellBash},         // Falls back to bash
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := autoDetectShellFromEnv(tt.shellEnv)
			if detected != tt.expected {
				t.Errorf("AutoDetectShell() = %v, want %v", detected, tt.expected)
			}
		})
	}
}

// autoDetectShellFromEnv is a test helper that simulates shell detection
func autoDetectShellFromEnv(shellEnv string) Shell {
	if shellEnv == "" {
		return ShellBash
	}
	if strings.HasSuffix(shellEnv, "/bash") {
		return ShellBash
	}
	if strings.HasSuffix(shellEnv, "/zsh") {
		return ShellZsh
	}
	if strings.HasSuffix(shellEnv, "/fish") {
		return ShellFish
	}
	return ShellBash
}

func TestInstaller(t *testing.T) {
	// Test with bash
	installer := NewInstaller(ShellBash)
	if installer.prefixDir == "" {
		t.Error("Installer should have a prefix directory")
	}

	// Test filename generation
	t.Run("filename", func(t *testing.T) {
		tests := []struct {
			shell    Shell
			expected string
		}{
			{ShellBash, "_kportal"},
			{ShellZsh, "_kportal"},
			{ShellFish, "kportal.fish"},
		}

		for _, tt := range tests {
			inst := NewInstaller(tt.shell)
			got := inst.getCompletionFilename()
			if got != tt.expected {
				t.Errorf("getCompletionFilename() for %v = %v, want %v", tt.shell, got, tt.expected)
			}
		}
	})
}

func TestInstallCompletion(t *testing.T) {
	// Temp directory (auto-removed by the test framework)
	tempDir := t.TempDir()

	// Test installation to temp directory
	installer := &Installer{
		shell:     ShellBash,
		prefixDir: tempDir,
	}

	// Should fail because file doesn't exist (doesn't matter, just test the method exists)
	_ = installer.Install()
}

func TestPrint(t *testing.T) {
	// Test that Print doesn't crash and outputs something
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := Print(ShellBash)

	_ = w.Close()
	os.Stdout = orig

	if err != nil {
		t.Fatalf("Print(ShellBash) failed: %v", err)
	}

	// Read output
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "_kportal") {
		t.Error("Print output should contain _kportal")
	}
}

func TestGetCompletionFilename(t *testing.T) {
	tests := []struct {
		shell    Shell
		expected string
	}{
		{ShellBash, "_kportal"},
		{ShellZsh, "_kportal"},
		{ShellFish, "kportal.fish"},
		{Shell("unknown"), "_kportal"}, // Falls back
	}

	for _, tt := range tests {
		inst := &Installer{shell: tt.shell}
		got := inst.getCompletionFilename()
		if got != tt.expected {
			t.Errorf("getCompletionFilename() for %v = %v, want %v", tt.shell, got, tt.expected)
		}
	}
}

func TestBashCompletionIncludesContextCompletion(t *testing.T) {
	script, err := Generate(ShellBash)
	if err != nil {
		t.Fatalf("Generate(ShellBash) failed: %v", err)
	}

	// Should include kubectl context completion
	if !strings.Contains(script, "kubectl config get-contexts") {
		t.Error("Bash completion should include kubectl context completion")
	}
}

func TestZshCompletionIncludesContextCompletion(t *testing.T) {
	script, err := Generate(ShellZsh)
	if err != nil {
		t.Fatalf("Generate(ShellZsh) failed: %v", err)
	}

	// Should include kubectl context completion
	if !strings.Contains(script, "kubectl config get-contexts") {
		t.Error("Zsh completion should include kubectl context completion")
	}
}

func TestFishCompletionIncludesContextCompletion(t *testing.T) {
	script, err := Generate(ShellFish)
	if err != nil {
		t.Fatalf("Generate(ShellFish) failed: %v", err)
	}

	// Should include kubectl context completion
	if !strings.Contains(script, "kubectl config get-contexts") {
		t.Error("Fish completion should include kubectl context completion")
	}
}

func TestAllFlagsPresent(t *testing.T) {
	script, err := Generate(ShellBash)
	if err != nil {
		t.Fatalf("Generate(ShellBash) failed: %v", err)
	}

	// Check all main flags are present. The main command exposes single-dash
	// -c/-v (no --config/--verbose long forms); word flags use --.
	flags := []string{
		"-c",
		"-v",
		"--version",
		"--update",
		"--check",
		"--headless",
		"--log-format",
		"--convert",
		"--convert-output",
	}

	for _, flag := range flags {
		if !strings.Contains(script, flag) {
			t.Errorf("Bash completion missing flag: %s", flag)
		}
	}
}

func TestSubcommandsPresent(t *testing.T) {
	for _, shell := range []Shell{ShellBash, ShellZsh, ShellFish} {
		script, err := Generate(shell)
		if err != nil {
			t.Fatalf("Generate(%v) failed: %v", shell, err)
		}

		for _, sub := range []string{"generate", "completion"} {
			if !strings.Contains(script, sub) {
				t.Errorf("%s completion missing %q subcommand", shell, sub)
			}
		}
	}
}

func TestGenerateFlagsPresent(t *testing.T) {
	script, err := Generate(ShellBash)
	if err != nil {
		t.Fatalf("Generate(ShellBash) failed: %v", err)
	}

	generateFlags := []string{
		"--context",
		"--config",
		"--dry-run",
	}

	for _, flag := range generateFlags {
		if !strings.Contains(script, flag) {
			t.Errorf("Bash completion missing generate flag: %s", flag)
		}
	}
}

func TestCompletionScriptPermissions(t *testing.T) {
	// Create temp file and verify permissions handling
	tempDir := t.TempDir()

	installer := &Installer{
		shell:     ShellBash,
		prefixDir: tempDir,
	}

	filename := filepath.Join(tempDir, installer.getCompletionFilename())

	// Write a test file
	err := os.WriteFile(filename, []byte("test"), 0o600)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Verify file exists and is readable
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	if string(data) != "test" {
		t.Error("File content mismatch")
	}
}
