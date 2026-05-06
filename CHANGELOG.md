# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased] - 2026-05-06

### Added
- `kportal generate --context=NAME [--config=PATH] [--dry-run]` subcommand for interactive bulk-add of forwards from a cluster. Walks namespace multi-select, service multi-select, and starting-port input; assigns consecutive local ports; emits one forward per port for multi-port services. Non-TCP ports are skipped and already-configured services are greyed out.
- HTTP log toggle in the add/edit wizard. Pressing `h` on the confirmation step toggles `httpLog: true/false` for the forward being added or edited. Advanced `httpLog` configuration set in YAML (`logFile`, `includeHeaders`, `maxBodySize`, `filterPath`) is preserved across edits.
- HTTP log header redaction. When `httpLog.includeHeaders: true`, sensitive headers (`Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, `X-Auth-Token`, `X-Csrf-Token`, `Proxy-Authorization`, `X-Access-Token`, plus any header whose name contains `token`/`secret`/`password`/`apikey`) have their values replaced with `[REDACTED]`. The header name is preserved. Always on, no opt-out.
- `install.sh` SHA-256 checksum verification. Every install verifies the downloaded archive against the release's `checksums.txt`. If `cosign` is on `PATH`, the checksums file's keyless cosign signature is also verified against the shared-actions reusable workflow identity. Set `DRY_RUN=1` to preview, `SKIP_COSIGN=1` to bypass cosign.

### Changed
- Headless mode (`kportal -headless`) now sends both structured and stdlib logs to stderr by default instead of `io.Discard`. `-v` still controls level (debug vs info), not destination.
- Context-name validator now permits common kubeconfig identifiers containing `@`, `.`, `:`, or `/` (e.g. `admin@home`, `user@cluster.example.com`, GKE dotted names, EKS ARNs).
- Edit-mode wizard now allows keeping the same local port. The port-availability check no longer rejects a forward's own port when editing it.

### Fixed
- `Esc` in the delete-confirmation dialog now cancels instead of confirming deletion (previously a data-loss bug).
- `Manager.Stop()` is now idempotent. Sequential or concurrent double-Stop no longer panics.
- Cosign cert-identity is now pinned to the actual signing workflow (`lukaszraczylo/shared-actions/.github/workflows/go-release.yaml@refs/heads/main`); previously cosign verification always failed.
- Internal concurrency races in the forward manager (`currentConfig` access under lock, `rest.Config` copied before mutation, `ForwardWorker.Stop` wrapped in `sync.Once`, `Reload` no longer kills the health checker). No user-visible flag, but resolves panics some users hit.

## [0.1.5] - 2025-11-23

### Added
- Interactive TUI built with Bubble Tea
- Real-time health check monitoring with grace period
- Toggle forwards on/off with Space key
- Error display below table showing detailed error messages
- Version display in UI title
- Complete log suppression for clean UI (klog included)
- Automatic error clearing when connection recovers

### Changed
- Replaced tview with Bubble Tea for better architecture
- Removed artificial 10-second delay before health checks
- Improved thread safety with message-passing architecture
- Enhanced status indicators (Active ●, Starting ○, Reconnecting ◐, Error ✗)

### Fixed
- Deadlock issues with tview UI
- Logs covering the legend in interactive mode
- Re-enable hang bug when toggling forwards
- Race conditions in status updates

## [0.1.0] - 2025-11-22

### Added
- Initial release
- Multi-context and multi-namespace support
- Automatic pod restart handling with prefix matching
- Label selector support for dynamic pod selection
- Hot-reload configuration watching
- Exponential backoff retry logic (max 10s)
- Port conflict detection with PID information
- kftray JSON to kportal YAML converter
- Alias support for cleaner display names
- Health check system
- Verbose and interactive modes
- Configuration validation
- Comprehensive test suite

[Unreleased]: https://github.com/lukaszraczylo/kportal/compare/v0.1.5...HEAD
[0.1.5]: https://github.com/lukaszraczylo/kportal/compare/v0.1.0...v0.1.5
[0.1.0]: https://github.com/lukaszraczylo/kportal/releases/tag/v0.1.0
