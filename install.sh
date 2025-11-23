#!/bin/bash

set -e

# kportal installation script
# Usage: curl -fsSL https://raw.githubusercontent.com/lukaszraczylo/kportal/main/install.sh | bash

REPO="lukaszraczylo/kportal"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print functions
print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "linux";;
        Darwin*)    echo "darwin";;
        MINGW*|MSYS*|CYGWIN*) echo "windows";;
        *)          echo "unknown";;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64";;
        aarch64|arm64)  echo "arm64";;
        armv7l)         echo "arm";;
        *)              echo "unknown";;
    esac
}

# Get latest version from GitHub
get_latest_version() {
    curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" |
        grep '"tag_name":' |
        sed -E 's/.*"v([^"]+)".*/\1/'
}

# Main installation
main() {
    echo ""
    echo "╔════════════════════════════════════════╗"
    echo "║   kportal Installation Script          ║"
    echo "║   Kubernetes Port Forwarding Made Easy ║"
    echo "╚════════════════════════════════════════╝"
    echo ""

    # Detect system
    OS=$(detect_os)
    ARCH=$(detect_arch)

    if [ "$OS" = "unknown" ] || [ "$ARCH" = "unknown" ]; then
        print_error "Unsupported operating system or architecture"
        print_info "OS: $(uname -s), Arch: $(uname -m)"
        exit 1
    fi

    print_info "Detected: ${OS}/${ARCH}"

    # Get latest version
    print_info "Fetching latest version..."
    VERSION=$(get_latest_version)

    if [ -z "$VERSION" ]; then
        print_error "Failed to fetch latest version"
        exit 1
    fi

    print_success "Latest version: v${VERSION}"

    # Construct download URL
    if [ "$OS" = "windows" ]; then
        ARCHIVE="kportal-${VERSION}-${OS}-${ARCH}.zip"
    else
        ARCHIVE="kportal-${VERSION}-${OS}-${ARCH}.tar.gz"
    fi

    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ARCHIVE}"

    # Create temporary directory
    TMP_DIR=$(mktemp -d)
    trap "rm -rf ${TMP_DIR}" EXIT

    # Download binary
    print_info "Downloading kportal..."
    if ! curl -fsSL -o "${TMP_DIR}/${ARCHIVE}" "${DOWNLOAD_URL}"; then
        print_error "Failed to download kportal"
        print_info "URL: ${DOWNLOAD_URL}"
        exit 1
    fi

    # Extract archive
    print_info "Extracting archive..."
    cd "${TMP_DIR}"
    if [ "$OS" = "windows" ]; then
        unzip -q "${ARCHIVE}"
        BINARY="kportal.exe"
    else
        tar -xzf "${ARCHIVE}"
        BINARY="kportal"
    fi

    # Check if binary exists
    if [ ! -f "${BINARY}" ]; then
        print_error "Binary not found after extraction"
        exit 1
    fi

    # Make binary executable
    chmod +x "${BINARY}"

    # Install binary
    print_info "Installing kportal to ${INSTALL_DIR}..."

    # Check if we need sudo
    if [ ! -w "${INSTALL_DIR}" ]; then
        print_warning "Installation directory requires sudo access"
        if command -v sudo >/dev/null 2>&1; then
            sudo mv "${BINARY}" "${INSTALL_DIR}/${BINARY}"
        else
            print_error "sudo not found. Please run with appropriate permissions"
            exit 1
        fi
    else
        mv "${BINARY}" "${INSTALL_DIR}/${BINARY}"
    fi

    # Verify installation
    if command -v kportal >/dev/null 2>&1; then
        INSTALLED_VERSION=$(kportal --version | grep -oP 'kportal version \K[0-9.]+' || echo "unknown")
        print_success "kportal v${INSTALLED_VERSION} installed successfully!"
    else
        print_warning "kportal installed but not found in PATH"
        print_info "You may need to add ${INSTALL_DIR} to your PATH"
    fi

    echo ""
    print_success "Installation complete!"
    echo ""
    echo "Get started:"
    echo "  1. Create a config file: touch .kportal.yaml"
    echo "  2. Run: kportal"
    echo ""
    echo "Documentation: https://lukaszraczylo.github.io/kportal"
    echo ""
}

main "$@"
