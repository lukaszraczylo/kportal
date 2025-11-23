# kportal

A robust Kubernetes port-forwarding tool that manages multiple concurrent port-forwards across different contexts, namespaces, and resources with automatic reconnection and failure recovery.

## Features

- **Multi-Context Support**: Forward ports from multiple Kubernetes contexts simultaneously
- **Automatic Pod Restart Handling**: Detects and reconnects to pods when they restart
- **Label Selector Support**: Dynamically target pods using label selectors
- **Prefix Matching**: Automatically find and reconnect to pods with name prefixes
- **Hot-Reload**: Configuration file changes are automatically detected and applied
- **Resilient Connections**: Infinite retry with exponential backoff (max 10s)
- **Port Conflict Detection**: Validates port availability before starting
- **Multiple Ports Per Resource**: Forward multiple ports from the same pod/service

## Installation

```bash
# Install development tools (including semver-gen for version management)
make install-tools

# Build from source (version automatically generated from git history)
make build

# Install to user bin directory
make install

# Install system-wide (requires sudo)
sudo make install-system

# Or build manually
go build -o kportal ./cmd/kportal
```

## Usage

### Basic Usage

```bash
# Use default config file (.kportal.yaml)
./kportal

# Use custom config file
./kportal -c myconfig.yaml

# Enable verbose logging
./kportal -v

# Validate configuration without starting
./kportal --check

# Convert kftray JSON config to kportal YAML
./kportal --convert configs.json --convert-output .kportal.yaml
```

### Configuration File

Create a `.kportal.yaml` file in your current directory:

```yaml
contexts:
  - name: production
    namespaces:
      - name: default
        forwards:
          # Pod with prefix matching (auto-handles restarts)
          - resource: pod/my-app
            protocol: tcp
            port: 8080
            localPort: 8080
            alias: my-api  # Optional: cleaner log output

          # Service forwarding with alias
          - resource: service/postgres
            protocol: tcp
            port: 5432
            localPort: 5432
            alias: prod-db

      - name: monitoring
        forwards:
          # Pod with label selector
          - resource: pod
            selector: app=prometheus
            protocol: tcp
            port: 9090
            localPort: 9090
            alias: prometheus

  - name: staging
    namespaces:
      - name: default
        forwards:
          # Multiple ports from same pod
          - resource: pod/test-app
            port: 8080
            localPort: 8081
            alias: test-http

          - resource: pod/test-app
            port: 9090
            localPort: 9091
            alias: test-metrics
```

### Resource Types

#### Pod with Prefix Matching
```yaml
- resource: pod/my-app    # Matches my-app-xyz789, my-app-abc123, etc.
  port: 8080
  localPort: 8080
```
Automatically reconnects to new pods when they restart.

#### Pod with Label Selector
```yaml
- resource: pod
  selector: app=nginx,env=prod
  port: 80
  localPort: 8080
```
Dynamically selects pods matching the label selector.

#### Service
```yaml
- resource: service/postgres
  port: 5432
  localPort: 5432
```
Most stable option - forwards to service endpoints.

#### Using Aliases

Aliases provide cleaner, more readable log output:

```yaml
- resource: service/victoria-metrics-cluster-vmselect
  port: 8481
  localPort: 8481
  alias: vmetrics  # Shows "vmetrics:8481→8481" instead of full path
```

**Without alias:**
```
[home/monitoring/service/victoria-metrics-cluster-vmselect:8481] Forwarding...
```

**With alias:**
```
[vmetrics:8481] Forwarding vmetrics:8481→8481 → localhost:8481
```

### Converting from kftray

kportal can automatically convert kftray JSON configurations to kportal YAML format:

```bash
# Convert kftray config
kportal --convert kftray-config.json --convert-output .kportal.yaml

# The converter will:
# 1. Read the kftray JSON format
# 2. Group forwards by context and namespace
# 3. Generate kportal YAML with all aliases preserved
# 4. Display a summary of the conversion
```

**Example kftray JSON:**
```json
[
  {
    "service": "postgres",
    "namespace": "default",
    "local_port": 5432,
    "remote_port": 5432,
    "context": "production",
    "workload_type": "service",
    "protocol": "tcp",
    "alias": "prod-db"
  }
]
```

**Converts to kportal YAML:**
```yaml
contexts:
  - name: production
    namespaces:
      - name: default
        forwards:
          - resource: service/postgres
            protocol: tcp
            port: 5432
            localPort: 5432
            alias: prod-db
```

## How It Works

### Pod Restart Handling

When a pod restarts:
1. The port-forward connection breaks
2. kportal immediately attempts to re-resolve the resource
3. For prefix matches: finds the newest pod with that prefix
4. For selectors: re-queries pods with matching labels
5. Reconnects to the new pod
6. Logs the switch: `Switched to new pod: old-pod → new-pod`

### Retry Strategy

Backoff intervals: **1s → 2s → 4s → 8s → 10s (max)**

- Connection failures trigger immediate resource re-resolution
- Retries continue indefinitely until successful
- Each forward has independent retry logic

### Hot-Reload

The config file is watched for changes:
1. File change detected
2. New config loaded and validated
3. Changes diff'd against current state
4. New forwards started, removed forwards stopped
5. Unchanged forwards continue running

If validation fails, the previous configuration remains active.

## Development

### Build Commands

```bash
# Build binary
make build

# Check current version (from semver-gen)
make version

# Run all checks (fmt, vet, staticcheck, test, build)
make all

# Run tests with race detection
make test

# Run code quality checks
make vet
make staticcheck
make fmt

# Install development tools (staticcheck, mockery, semver-gen)
make install-tools

# Generate test coverage report
make coverage
```

### Semantic Versioning

This project uses [semver-gen](https://github.com/lukaszraczylo/semver-generator) for automatic semantic version generation based on git commit messages.

**Version Keywords:**
- **Patch** (0.0.X): `fix`, `bugfix`, `hotfix`, `patch`, `docs`, `test`, `refactor`
- **Minor** (0.X.0): `feat`, `feature`, `add`, `enhance`, `update`, `improve`
- **Major** (X.0.0): `breaking`, `major`, `BREAKING CHANGE`

The version is automatically calculated from your git history and embedded in the binary at build time.

```bash
# Check current version
make version

# Build with auto-generated version
make build

# Verify version in binary
./kportal --version
```

Configuration is managed in `semver.yaml`. To manually install semver-gen:

```bash
# Automatically installed via make install-tools
# Or install manually from https://github.com/lukaszraczylo/semver-generator
```

### Project Structure

```
kportal/
├── cmd/kportal/          # CLI entry point
├── internal/
│   ├── config/           # Configuration parsing and validation
│   ├── forward/          # Port-forward workers and manager
│   ├── k8s/             # Kubernetes client, resolver, port-forward wrapper
│   └── retry/           # Exponential backoff logic
├── test/
│   ├── integration/     # Integration tests
│   ├── fixtures/        # Test configurations
│   └── helpers/         # Test utilities
├── .kportal.yaml        # Example configuration
├── semver.yaml          # Semantic version configuration
├── Makefile             # Build automation
└── CLAUDE.md            # Development guide
```

## Signal Handling

- `CTRL+C` / `SIGTERM`: Graceful shutdown (closes all forwards)
- `SIGHUP`: Reload configuration file

## Port Conflict Detection

kportal validates ports at multiple stages:

1. **Config Parse Time**: Detects duplicate local ports in configuration
2. **Startup Time**: Checks if ports are available on the system
3. **Hot-Reload Time**: Validates new ports before applying changes

Errors show which process is using conflicting ports (with PID).

## Examples

### Forward Multiple Services from Production

```yaml
contexts:
  - name: production
    namespaces:
      - name: default
        forwards:
          - resource: service/api
            port: 8080
            localPort: 8080
          - resource: service/postgres
            port: 5432
            localPort: 5432
          - resource: service/redis
            port: 6379
            localPort: 6379
```

### Monitor Multiple Environments

```yaml
contexts:
  - name: production
    namespaces:
      - name: monitoring
        forwards:
          - resource: service/prometheus
            port: 9090
            localPort: 9090

  - name: staging
    namespaces:
      - name: monitoring
        forwards:
          - resource: service/prometheus
            port: 9090
            localPort: 9091  # Different local port
```

### Debug Specific Pods

```yaml
contexts:
  - name: production
    namespaces:
      - name: default
        forwards:
          # Forward app HTTP and debug ports
          - resource: pod
            selector: app=myapp,version=v2
            port: 8080
            localPort: 8080

          - resource: pod
            selector: app=myapp,version=v2
            port: 6060  # pprof
            localPort: 6060
```

## License

MIT

## Contributing

Contributions welcome! Please ensure:
- Code passes `make check` (fmt, vet, staticcheck)
- Tests pass with `make test`
- New features include tests
