#!/bin/bash

set -e

# kportal installation script
# Usage: curl -fsSL https://raw.githubusercontent.com/lukaszraczylo/kportal/main/install.sh | bash
#
# Environment overrides:
#   INSTALL_DIR  - target install directory (default: /usr/local/bin)
#   KPORTAL_VERSION - install a specific version instead of latest (e.g. 1.2.3)
#   DRY_RUN=1    - download and verify but do not install (for local testing)
#   SKIP_COSIGN=1 - skip cosign signature verification even if cosign is present

REPO="lukaszraczylo/kportal"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
DRY_RUN="${DRY_RUN:-0}"
SKIP_COSIGN="${SKIP_COSIGN:-0}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print functions
print_info() {
    echo -e "${BLUE}i${NC} $1"
}

print_success() {
    echo -e "${GREEN}OK${NC} $1"
}

print_error() {
    echo -e "${RED}X${NC} $1" >&2
}

print_warning() {
    echo -e "${YELLOW}!${NC} $1"
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

# Compute sha256 of a file. Uses shasum which is available on macOS and Linux.
compute_sha256() {
    local file="$1"
    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "${file}" | awk '{ print $1 }'
    elif command -v sha256sum >/dev/null 2>&1; then
        sha256sum "${file}" | awk '{ print $1 }'
    else
        print_error "Neither 'shasum' nor 'sha256sum' is available; cannot verify checksum"
        exit 1
    fi
}

# Verify the archive against checksums.txt (SHA-256). Aborts on mismatch.
verify_checksum() {
    local archive="$1"
    local checksums_file="$2"

    print_info "Verifying SHA-256 checksum..."

    local expected
    # Match the archive name as the second whitespace-separated field.
    # checksums.txt format produced by goreleaser: "<sha256>  <filename>"
    expected=$(awk -v name="${archive}" '$2 == name { print $1; exit }' "${checksums_file}")

    if [ -z "${expected}" ]; then
        print_error "Checksum for ${archive} not found in checksums.txt"
        print_error "Refusing to install unverified binary."
        exit 1
    fi

    local actual
    actual=$(compute_sha256 "${archive}")

    if [ "${expected}" != "${actual}" ]; then
        print_error "Checksum mismatch for ${archive}"
        print_error "  expected: ${expected}"
        print_error "  actual:   ${actual}"
        print_error "Aborting installation. The downloaded archive may be corrupted or tampered with."
        exit 1
    fi

    print_success "SHA-256 checksum OK"
}

# Optional: verify cosign signature on the checksums file. Silently skipped
# when cosign is not installed or the signature artefact is not present.
verify_cosign_signature() {
    local checksums_file="$1"
    local sig_file="$2"

    if [ "${SKIP_COSIGN}" = "1" ]; then
        return 0
    fi

    if ! command -v cosign >/dev/null 2>&1; then
        # cosign not installed; supply-chain integrity still rests on SHA-256
        return 0
    fi

    if [ ! -f "${sig_file}" ]; then
        # No sig artefact downloaded; skip silently
        return 0
    fi

    print_info "Verifying cosign signature on checksums.txt..."
    if cosign verify-blob \
        --certificate-identity-regexp "https://github.com/${REPO}/.*" \
        --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
        --bundle "${sig_file}" \
        "${checksums_file}" >/dev/null 2>&1; then
        print_success "cosign signature OK"
    else
        print_error "cosign signature verification FAILED for checksums.txt"
        print_error "Aborting installation."
        exit 1
    fi
}

# Main installation
main() {
    echo ""
    echo "kportal installation script"
    echo "Kubernetes port forwarding made easy"
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

    # Get version
    if [ -n "${KPORTAL_VERSION:-}" ]; then
        VERSION="${KPORTAL_VERSION#v}"
        print_info "Using requested version: v${VERSION}"
    else
        print_info "Fetching latest version..."
        VERSION=$(get_latest_version)
        if [ -z "$VERSION" ]; then
            print_error "Failed to fetch latest version"
            exit 1
        fi
        print_success "Latest version: v${VERSION}"
    fi

    # Construct download URLs
    if [ "$OS" = "windows" ]; then
        ARCHIVE="kportal-${VERSION}-${OS}-${ARCH}.zip"
    else
        ARCHIVE="kportal-${VERSION}-${OS}-${ARCH}.tar.gz"
    fi

    BASE_URL="https://github.com/${REPO}/releases/download/v${VERSION}"
    DOWNLOAD_URL="${BASE_URL}/${ARCHIVE}"
    CHECKSUMS_FILE="kportal-${VERSION}-checksums.txt"
    CHECKSUMS_URL="${BASE_URL}/${CHECKSUMS_FILE}"
    SIG_FILE="${CHECKSUMS_FILE}.sigstore.json"
    SIG_URL="${BASE_URL}/${SIG_FILE}"

    # Create temporary directory
    TMP_DIR=$(mktemp -d)
    # shellcheck disable=SC2064
    trap "rm -rf '${TMP_DIR}'" EXIT

    # Download archive
    print_info "Downloading ${ARCHIVE}..."
    if ! curl -fsSL -o "${TMP_DIR}/${ARCHIVE}" "${DOWNLOAD_URL}"; then
        print_error "Failed to download kportal archive"
        print_info "URL: ${DOWNLOAD_URL}"
        exit 1
    fi

    # Download checksums
    print_info "Downloading checksums.txt..."
    if ! curl -fsSL -o "${TMP_DIR}/${CHECKSUMS_FILE}" "${CHECKSUMS_URL}"; then
        print_error "Failed to download checksums file"
        print_info "URL: ${CHECKSUMS_URL}"
        print_error "Refusing to install without checksum verification."
        exit 1
    fi

    # Try to download cosign signature bundle (best-effort, non-fatal if absent)
    if curl -fsSL -o "${TMP_DIR}/${SIG_FILE}" "${SIG_URL}" 2>/dev/null; then
        :
    else
        rm -f "${TMP_DIR}/${SIG_FILE}"
    fi

    # Verify archive checksum
    cd "${TMP_DIR}"
    verify_checksum "${ARCHIVE}" "${CHECKSUMS_FILE}"

    # Optional cosign signature verification on checksums file
    verify_cosign_signature "${CHECKSUMS_FILE}" "${SIG_FILE}"

    # Extract archive
    print_info "Extracting archive..."
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

    if [ "${DRY_RUN}" = "1" ]; then
        print_success "Dry run successful. Verified archive at ${TMP_DIR}/${ARCHIVE}"
        print_info "Skipping install step (DRY_RUN=1)"
        return 0
    fi

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

    # Verify installation (portable: awk instead of GNU-only grep -oP)
    if command -v kportal >/dev/null 2>&1; then
        INSTALLED_VERSION=$(kportal --version 2>/dev/null | awk '/^kportal version/ { print $3; exit }')
        if [ -z "${INSTALLED_VERSION}" ]; then
            INSTALLED_VERSION="unknown"
        fi
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
