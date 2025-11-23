# Contributing to kportal

Thank you for your interest in contributing to kportal! This document provides guidelines and instructions for contributing.

## Code of Conduct

Be respectful and constructive. We're all here to build something awesome together.

## How to Contribute

### Reporting Bugs

Before creating a bug report, please check if the issue already exists. When creating a bug report, include:

- **Clear title and description**
- **Steps to reproduce**
- **Expected behavior**
- **Actual behavior**
- **Screenshots** (if applicable)
- **Environment details** (OS, kportal version, Go version, Kubernetes version)
- **Configuration file** (sanitized)

### Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. When creating an enhancement suggestion, include:

- **Clear title and description**
- **Use case** - why this enhancement would be useful
- **Possible implementation** (optional)
- **Alternative solutions** (if any)

### Pull Requests

1. **Fork the repository**
   ```bash
   git clone https://github.com/yourusername/kportal.git
   cd kportal
   ```

2. **Create a feature branch**
   ```bash
   git checkout -b feature/amazing-feature
   ```

3. **Make your changes**
   - Write clear, readable code
   - Follow Go conventions and best practices
   - Add tests for new functionality
   - Update documentation as needed

4. **Run quality checks**
   ```bash
   make all  # Runs fmt, vet, staticcheck, test, build
   ```

5. **Commit your changes**

   Follow [semantic commit messages](#commit-message-format):
   ```bash
   git commit -m "feat: add amazing feature"
   ```

6. **Push to your fork**
   ```bash
   git push origin feature/amazing-feature
   ```

7. **Open a Pull Request**
   - Provide a clear description of the changes
   - Reference any related issues
   - Ensure all CI checks pass

## Development Setup

### Prerequisites

- Go 1.23 or higher
- Access to a Kubernetes cluster
- kubectl configured

### Building

```bash
# Install development tools
make install-tools

# Build the binary
make build

# Run tests
make test

# Run with race detection
go test -race ./...

# Check code quality
make vet
make staticcheck
make fmt
```

### Running Locally

```bash
# Build and install
make build
make install

# Run with your config
kportal -c .kportal.yaml

# Run in verbose mode
kportal -v
```

## Commit Message Format

We use semantic commit messages for automatic version generation:

### Commit Types

- **feat** - New feature (bumps minor version)
- **fix** - Bug fix (bumps patch version)
- **docs** - Documentation only changes (bumps patch version)
- **style** - Code style changes (formatting, missing semi colons, etc.)
- **refactor** - Code refactoring
- **test** - Adding or updating tests
- **chore** - Maintenance tasks
- **breaking** - Breaking changes (bumps major version)

### Format

```
<type>: <description>

[optional body]

[optional footer]
```

### Examples

```bash
# Feature
git commit -m "feat: add health check grace period"

# Bug fix
git commit -m "fix: resolve port conflict detection bug"

# Breaking change
git commit -m "breaking: change config file format

BREAKING CHANGE: Config file format has changed from JSON to YAML.
Migration tool available with --convert flag."

# Documentation
git commit -m "docs: update installation instructions"
```

## Project Structure

```
kportal/
├── cmd/kportal/              # Main application entry point
├── internal/
│   ├── config/               # Configuration parsing and validation
│   ├── forward/              # Port-forward manager and workers
│   ├── healthcheck/          # Health monitoring system
│   ├── k8s/                  # Kubernetes client wrapper
│   ├── retry/                # Retry logic with backoff
│   ├── ui/                   # Terminal UI implementations
│   └── converter/            # kftray JSON converter
├── Formula/                  # Homebrew formula
├── .github/workflows/        # CI/CD pipelines
└── docs/                     # Documentation and GitHub Pages
```

## Coding Guidelines

### Go Style

- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting
- Keep functions small and focused
- Write descriptive variable names
- Add comments for exported functions
- Handle errors explicitly

### Testing

- Write tests for new functionality
- Aim for meaningful test coverage
- Use table-driven tests where appropriate
- Mock external dependencies
- Test edge cases and error conditions

Example test:
```go
func TestForwardWorker_Start(t *testing.T) {
    tests := []struct {
        name    string
        forward config.Forward
        wantErr bool
    }{
        {
            name: "valid pod forward",
            forward: config.Forward{
                Resource:  "pod/test",
                Port:      8080,
                LocalPort: 8080,
            },
            wantErr: false,
        },
        // Add more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Documentation

- Update README.md for user-facing changes
- Add code comments for complex logic
- Update CHANGELOG.md following [Keep a Changelog](https://keepachangelog.com/)
- Document new configuration options
- Add examples for new features

## Release Process

Releases are automated via GitHub Actions:

1. **Version is determined automatically** by semver-gen based on commit messages
2. **Create a tag** following semantic versioning:
   ```bash
   git tag -a v0.2.0 -m "Release version 0.2.0"
   git push origin v0.2.0
   ```
3. **GitHub Actions will**:
   - Build binaries for all platforms
   - Create GitHub release
   - Update Homebrew formula
   - Generate release notes

## Getting Help

- **Questions?** Open a [GitHub Discussion](https://github.com/lukaszraczylo/kportal/discussions)
- **Bug or feature request?** Open a [GitHub Issue](https://github.com/lukaszraczylo/kportal/issues)
- **Want to chat?** Reach out on GitHub

## Recognition

Contributors will be recognized in:
- GitHub's contributor graph
- Release notes
- README.md (for significant contributions)

Thank you for contributing to kportal! 🎉
