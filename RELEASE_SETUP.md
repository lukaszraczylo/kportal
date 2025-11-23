# Release Infrastructure Setup Summary

This document summarizes all the release infrastructure that has been set up for kportal.

## ✅ Completed Setup

### 1. GitHub Actions CI/CD Pipeline

**File**: `.github/workflows/release.yml`

**Features**:
- Multi-platform binary builds (Linux, macOS, Windows - amd64 & arm64)
- Automatic release creation on version tags
- Binary archiving (tar.gz for Unix, zip for Windows)
- SHA256 checksum generation
- Automated Homebrew formula updates
- Release notes generation

**How to trigger**:
```bash
# Commit with semantic versioning keywords
git commit -m "feat: add new feature"

# Tag the release
git tag -a v0.2.0 -m "Release v0.2.0"

# Push tags
git push origin v0.2.0
```

The pipeline will automatically:
1. Build binaries for all platforms
2. Create GitHub release with binaries
3. Update Homebrew tap formula
4. Generate release notes

### 2. Installation Methods

#### A. Homebrew Formula

**File**: `Formula/kportal.rb`

**Installation command**:
```bash
brew install lukaszraczylo/tap/kportal
```

**Note**: Formula is automatically updated by CI/CD pipeline. You'll need to create a separate tap repository:
1. Create repo: `https://github.com/lukaszraczylo/brew-taps`
2. Add Formula/kportal.rb to that repo
3. Set `HOMEBREW_TAP_TOKEN` secret in GitHub repository settings

#### B. Quick Install Script

**File**: `install.sh`

**Features**:
- Auto-detects OS and architecture
- Downloads appropriate binary
- Extracts and installs to /usr/local/bin
- Verifies installation
- Colorful output with emoji indicators

**Installation command**:
```bash
curl -fsSL https://raw.githubusercontent.com/lukaszraczylo/kportal/main/install.sh | bash
```

#### C. Manual Download

Users can download binaries directly from GitHub releases:
```
https://github.com/lukaszraczylo/kportal/releases
```

### 3. Documentation

#### A. Comprehensive README.md

**File**: `README.md`

**Contents**:
- Feature showcase with emojis
- Multiple installation methods
- Quick start guide
- Configuration examples
- Usage instructions
- Advanced features documentation
- Troubleshooting guide
- Contributing guidelines

#### B. GitHub Pages Website

**File**: `docs/index.html`

**Features**:
- Modern, responsive design with TailwindCSS
- Hero section with clear CTA
- Feature showcase cards
- Installation guide
- Configuration examples with syntax highlighting
- Documentation links
- Mobile-friendly

**URL** (once enabled): `https://lukaszraczylo.github.io/kportal`

**To enable**:
1. Go to GitHub repository settings
2. Pages section
3. Source: Deploy from a branch
4. Branch: main
5. Folder: /docs

### 4. Supporting Files

#### CHANGELOG.md
**File**: `CHANGELOG.md`

Tracks all changes following Keep a Changelog format. Update this file with each release.

#### CONTRIBUTING.md
**File**: `CONTRIBUTING.md`

Guidelines for:
- Bug reporting
- Feature requests
- Pull request process
- Commit message format
- Development setup
- Testing guidelines

## 🚀 Release Workflow

### Standard Release Process

1. **Develop features**
   ```bash
   git checkout -b feature/my-feature
   # Make changes
   make test
   make all
   ```

2. **Commit with semantic messages**
   ```bash
   git commit -m "feat: add amazing feature"
   git commit -m "fix: resolve bug in health check"
   ```

3. **Update CHANGELOG.md**
   ```markdown
   ## [0.2.0] - 2025-11-24
   
   ### Added
   - Amazing new feature
   
   ### Fixed
   - Bug in health check
   ```

4. **Tag the release**
   ```bash
   git tag -a v0.2.0 -m "Release v0.2.0"
   git push origin main
   git push origin v0.2.0
   ```

5. **CI/CD automatically**:
   - Builds all binaries
   - Creates GitHub release
   - Updates Homebrew formula
   - Attaches binaries and checksums

### Version Bumping (Semantic Versioning)

Version is automatically determined by semver-gen from commit messages:

- **Patch** (0.0.X): `fix`, `bugfix`, `hotfix`, `patch`, `docs`, `test`, `refactor`
- **Minor** (0.X.0): `feat`, `feature`, `add`, `enhance`, `update`, `improve`
- **Major** (X.0.0): `breaking`, `major`, `BREAKING CHANGE`

## 📦 Platform Support

### Supported Platforms

| OS      | Architecture | Archive Format |
|---------|-------------|----------------|
| Linux   | amd64       | tar.gz         |
| Linux   | arm64       | tar.gz         |
| macOS   | amd64       | tar.gz         |
| macOS   | arm64       | tar.gz         |
| Windows | amd64       | zip            |
| Windows | arm64       | zip            |

## 🔒 Required GitHub Secrets

For full automation, set these secrets in your GitHub repository:

1. **GITHUB_TOKEN** - Automatically provided by GitHub Actions
2. **HOMEBREW_TAP_TOKEN** - Personal access token for updating Homebrew tap
   - Create at: https://github.com/settings/tokens
   - Permissions needed: `repo` scope
   - Add to repository secrets

## 📝 Next Steps

### 1. Enable GitHub Pages
- Repository Settings → Pages → Source: main branch, /docs folder

### 2. Create Homebrew Tap Repository
```bash
# Create new repository
gh repo create lukaszraczylo/brew-taps --public

# Clone and set up
git clone https://github.com/lukaszraczylo/brew-taps
cd brew-taps
cp ../kportal/Formula/kportal.rb ./Formula/
git add Formula/kportal.rb
git commit -m "Initial formula for kportal"
git push origin main
```

### 3. Add GitHub Token to Secrets
- Repository Settings → Secrets and variables → Actions
- New repository secret
- Name: `HOMEBREW_TAP_TOKEN`
- Value: Your personal access token

### 4. Create First Release
```bash
cd kportal
git add .
git commit -m "feat: initial release setup"
git push origin main
git tag -a v0.1.5 -m "Release v0.1.5"
git push origin v0.1.5
```

### 5. Test Installation Methods

After first release, test:
```bash
# Homebrew (once tap is set up)
brew install lukaszraczylo/tap/kportal

# Quick install script
curl -fsSL https://raw.githubusercontent.com/lukaszraczylo/kportal/main/install.sh | bash

# Manual download
# Visit releases page and download binary
```

## 🎨 Customization

### Update Website Colors

Edit `docs/index.html`:
```javascript
tailwind.config = {
    theme: {
        extend: {
            colors: {
                primary: '#3b82f6',    // Blue
                secondary: '#8b5cf6',   // Purple
                dark: '#0f172a',        // Dark slate
            }
        }
    }
}
```

### Update Release Notes Template

Edit `.github/workflows/release.yml` in the "Generate release notes" step.

## 📊 Monitoring

After releases, monitor:
- GitHub Actions workflow runs
- GitHub Releases page
- Homebrew tap repository commits
- Download statistics on releases page

## 🐛 Troubleshooting

### Release workflow fails
- Check GitHub Actions logs
- Verify all required secrets are set
- Ensure tag follows v\d+.\d+.\d+ format

### Homebrew formula not updating
- Verify HOMEBREW_TAP_TOKEN is valid
- Check tap repository permissions
- Review release workflow logs

### Install script fails
- Test locally with different OS/arch combinations
- Check release binary naming matches script expectations
- Verify binaries are attached to release

## ✅ Checklist for First Release

- [ ] All code committed and pushed
- [ ] GitHub Pages enabled
- [ ] Homebrew tap repository created
- [ ] HOMEBREW_TAP_TOKEN secret set
- [ ] CHANGELOG.md updated
- [ ] Version tag created and pushed
- [ ] Release workflow completed successfully
- [ ] Binaries attached to release
- [ ] Homebrew formula updated
- [ ] Install script tested
- [ ] Documentation website live
- [ ] README.md installation links work

---

**Documentation last updated**: 2025-11-23
**Setup completed for**: kportal v0.1.5
