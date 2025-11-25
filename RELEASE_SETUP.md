# Release Infrastructure

Documentation for kportal's release automation and distribution.

## 🔄 CI/CD Pipeline

**File**: `.github/workflows/release.yml`

The pipeline builds multi-platform binaries, creates GitHub releases, and updates Homebrew on version tags.

### Trigger a Release

```bash
git tag -a v0.2.0 -m "Release v0.2.0"
git push origin v0.2.0
```

The pipeline will:
1. Build binaries for all platforms
2. Create GitHub release with binaries and checksums
3. Update Homebrew tap formula

## 📦 Installation Methods

### Homebrew

**File**: `Formula/kportal.rb`

```bash
brew install lukaszraczylo/tap/kportal
```

Formula is automatically updated by CI/CD. Requires:
- Tap repository: `https://github.com/lukaszraczylo/brew-taps`
- Secret: `HOMEBREW_TAP_TOKEN` with `repo` scope

### Install Script

**File**: `install.sh`

```bash
curl -fsSL https://raw.githubusercontent.com/lukaszraczylo/kportal/main/install.sh | bash
```

Auto-detects OS/architecture and installs to `/usr/local/bin`.

### Manual Download

Download from [releases page](https://github.com/lukaszraczylo/kportal/releases).

## Platform Support

| OS | Architecture | Format |
|----|--------------|--------|
| Linux | amd64, arm64 | tar.gz |
| macOS | amd64, arm64 | tar.gz |
| Windows | amd64, arm64 | zip |

## 🚀 Release Process

1. **Make changes and test**
   ```bash
   make test && make all
   ```

2. **Update CHANGELOG.md**

3. **Tag and push**
   ```bash
   git tag -a v0.2.0 -m "Release v0.2.0"
   git push origin main
   git push origin v0.2.0
   ```

## Version Bumping

Version determined by commit message keywords:

| Bump | Keywords |
|------|----------|
| Patch (0.0.X) | `fix`, `bugfix`, `docs`, `test`, `refactor` |
| Minor (0.X.0) | `feat`, `feature`, `add`, `enhance`, `update` |
| Major (X.0.0) | `breaking`, `major`, `BREAKING CHANGE` |

## Required Secrets

| Secret | Purpose |
|--------|---------|
| `GITHUB_TOKEN` | Provided by GitHub Actions |
| `HOMEBREW_TAP_TOKEN` | Personal access token with `repo` scope |

## ⚙️ Initial Setup

### 1. Enable GitHub Pages

Repository Settings → Pages → Source: main branch, /docs folder

### 2. Create Homebrew Tap

```bash
gh repo create lukaszraczylo/brew-taps --public
cd brew-taps
mkdir Formula
# Formula will be auto-updated by CI
```

### 3. Add Token Secret

Repository Settings → Secrets → Actions → New secret:
- Name: `HOMEBREW_TAP_TOKEN`
- Value: Personal access token with `repo` scope

## 🐛 Troubleshooting

### Release workflow fails
- Check GitHub Actions logs
- Verify secrets are configured
- Ensure tag follows `v\d+.\d+.\d+` format

### Homebrew not updating
- Verify `HOMEBREW_TAP_TOKEN` is valid
- Check tap repository permissions

### Install script fails
- Verify release binaries are attached
- Check binary naming matches script expectations
