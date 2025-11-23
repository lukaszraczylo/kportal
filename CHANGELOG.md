# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
