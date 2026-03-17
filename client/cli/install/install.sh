#!/usr/bin/env bash
set -euo pipefail

# VeilKey Guard Installer
#
# Downloads pre-built binary first, falls back to source build if unavailable.
#
# Environment:
#   VEILKEY_LOCALVAULT_URL  Preferred localvault URL after install
#   VEILKEY_API             Legacy fallback endpoint
#   VEILKEY_VERSION       Version to install (default: latest)
#   VEILKEY_DOWNLOAD_URL  Binary download base URL
#   VEILKEY_REPO_URL      Git repo URL (for source build fallback)
#   VEILKEY_INSTALL_DIR   Install path (default: /opt/veilkey)
#   VEILKEY_BRANCH        Branch (default: main)
#   GO_VERSION            Go version (default: 1.24.4)
#   NONINTERACTIVE=1      Non-interactive install (no prompts)

VERSION="${VEILKEY_VERSION:-v0.3.0}"
INSTALL_DIR="${VEILKEY_INSTALL_DIR:-/opt/veilkey}"
BRANCH="${VEILKEY_BRANCH:-main}"
GO_VERSION="${GO_VERSION:-1.24.4}"
DOWNLOAD_URL="${VEILKEY_DOWNLOAD_URL:-}"

echo "=== VeilKey Guard Install ==="
echo ""

# --- Helpers ---
ask_yn() {
    local prompt="$1" default="${2:-y}"
    if [[ "${NONINTERACTIVE:-}" == "1" ]]; then return 0; fi
    read -rp "$prompt [Y/n] " answer
    case "${answer:-$default}" in [yY]*) return 0 ;; *) return 1 ;; esac
}

detect_platform() {
    local os arch
    case "$(uname -s)" in
        Linux)  os="linux" ;;
        Darwin) os="darwin" ;;
        MINGW*|MSYS*|CYGWIN*) os="windows" ;;
        *) echo "ERROR: Unsupported OS: $(uname -s)" >&2; exit 1 ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *) echo "ERROR: Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
    esac
    echo "${os}-${arch}"
}

PLATFORM=$(detect_platform)
OS="${PLATFORM%-*}"
echo "Platform: ${PLATFORM}"
echo ""

# --- Method 1: Pre-built binary download ---
install_binary() {
    if [[ -z "$DOWNLOAD_URL" ]]; then
        return 1
    fi

    local ext="tar.gz"
    [[ "$OS" == "windows" ]] && ext="zip"
    local filename="veilkey-cli-${VERSION#v}-${PLATFORM}.${ext}"
    local url="${DOWNLOAD_URL}/${VERSION}/${filename}"

    echo "Downloading binary: ${url}"
    local tmpfile="/tmp/${filename}"
    if ! curl -sfL "$url" -o "$tmpfile"; then
        echo "  Download failed — falling back to source build"
        return 1
    fi

    local bindir="/usr/local/bin"
    [[ "$OS" == "windows" ]] && bindir="${INSTALL_DIR}/bin"

    mkdir -p "$bindir"

    if [[ "$ext" == "zip" ]]; then
        local tmpdir=$(mktemp -d)
        unzip -qo "$tmpfile" -d "$tmpdir"
        mv "$tmpdir"/veilkey-cli-*.exe "$bindir/veilkey-cli.exe"
        rm -rf "$tmpdir"
    else
        local tmpdir=$(mktemp -d)
        tar -xzf "$tmpfile" -C "$tmpdir"
        mv "$tmpdir/veilkey-cli-${PLATFORM}" "$bindir/veilkey-cli"
        chmod +x "$bindir/veilkey-cli"
        # Install scripts (Linux/macOS)
        [[ -f "$tmpdir/vk" ]] && mv "$tmpdir/vk" "$bindir/vk" && chmod +x "$bindir/vk"
        [[ -f "$tmpdir/vk_wrap" ]] && mv "$tmpdir/vk_wrap" "$bindir/vk_wrap" && chmod +x "$bindir/vk_wrap"
        rm -rf "$tmpdir"
    fi

    rm -f "$tmpfile"
    echo "  Binary install complete"
    return 0
}

# --- Method 2: Source build ---
install_from_source() {
    echo "Installing via source build"
    echo ""

    # Check git
    if ! command -v git &>/dev/null; then
        echo "git not found. Installing..."
        if command -v apt-get &>/dev/null; then
            apt-get update -qq && apt-get install -y -qq git
        elif command -v yum &>/dev/null; then
            yum install -y git
        elif command -v apk &>/dev/null; then
            apk add git
        elif command -v brew &>/dev/null; then
            brew install git
        else
            echo "ERROR: Please install git manually." >&2
            exit 1
        fi
    fi

    # Check Go
    if ! command -v go &>/dev/null; then
        echo "Go is not installed."
        if ask_yn "Install Go ${GO_VERSION} automatically?"; then
            install_go
        else
            echo "Aborted. Please install Go manually: https://go.dev/dl/" >&2
            exit 1
        fi
    else
        echo "Go: $(go version | awk '{print $3}')"
    fi

    # Check repo URL
    REPO_URL="${VEILKEY_REPO_URL:-}"
    if [[ -z "$REPO_URL" ]]; then
        if [[ "${NONINTERACTIVE:-}" == "1" ]]; then
            echo "ERROR: Set VEILKEY_REPO_URL environment variable." >&2
            exit 1
        fi
        read -rp "Enter Git repo URL: " REPO_URL
        if [[ -z "$REPO_URL" ]]; then
            echo "ERROR: Repo URL is required." >&2
            exit 1
        fi
    fi

    # Clone or update
    if [ -d "$INSTALL_DIR/.git" ]; then
        echo "Updating existing installation..."
        cd "$INSTALL_DIR"
        git fetch origin
        git checkout "$BRANCH"
        git pull origin "$BRANCH"
    else
        echo "Install directory: $INSTALL_DIR"
        [ -d "$INSTALL_DIR" ] && rm -rf "$INSTALL_DIR"
        git clone --depth 1 -b "$BRANCH" "${REPO_URL%.git}.git" "$INSTALL_DIR"
    fi

    # Build
    echo ""
    echo "Building..."
    cd "$INSTALL_DIR/guard"
    make build

    # Symlinks
    ln -sf "$INSTALL_DIR/guard/bin/veilkey-cli" /usr/local/bin/veilkey-cli
    echo "  linked: /usr/local/bin/veilkey-cli"

    if [ -f "$INSTALL_DIR/scripts/vk" ]; then
        ln -sf "$INSTALL_DIR/scripts/vk" /usr/local/bin/vk
        echo "  linked: /usr/local/bin/vk"
    fi
    if [ -f "$INSTALL_DIR/scripts/vk_wrap" ]; then
        ln -sf "$INSTALL_DIR/scripts/vk_wrap" /usr/local/bin/vk_wrap
        echo "  linked: /usr/local/bin/vk_wrap"
    fi
}

install_go() {
    local os_name arch
    case "$(uname -s)" in
        Linux)  os_name="linux" ;;
        Darwin) os_name="darwin" ;;
        *) echo "ERROR: Auto Go install only supports Linux/macOS." >&2; exit 1 ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *) echo "ERROR: Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
    esac

    local tarball="go${GO_VERSION}.${os_name}-${arch}.tar.gz"
    local url="https://go.dev/dl/${tarball}"

    echo "Downloading Go ${GO_VERSION} (${os_name}/${arch})..."
    curl -sfL "$url" -o "/tmp/${tarball}" || {
        echo "ERROR: Go download failed: $url" >&2; exit 1
    }

    echo "Installing Go..."
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"

    export PATH="/usr/local/go/bin:$PATH"

    if [[ "$os_name" == "linux" ]] && [[ -d /etc/profile.d ]]; then
        echo 'export PATH="/usr/local/go/bin:$PATH"' > /etc/profile.d/go.sh
    fi

    echo "  Go $(go version | awk '{print $3}') installed"
}

# --- Run install ---
if install_binary; then
    : # Binary install succeeded
else
    install_from_source
fi

# --- Output result ---
echo ""
echo "=== Installation complete ==="
echo ""

BIN="veilkey-cli"
[[ "$OS" == "windows" ]] && BIN="veilkey-cli.exe"

if command -v "$BIN" &>/dev/null; then
    echo "  $($BIN version 2>/dev/null)"
fi

echo ""
echo "Required setup:"
echo "  export VEILKEY_LOCALVAULT_URL=https://127.0.0.1:10180"
echo ""
echo "Usage:"
echo "  veilkey-cli status              Show status"
echo "  veilkey-cli wrap <command>       Run command + auto-replace secrets"
echo "  veilkey-cli scan <file>          Scan file for secrets"
echo "  veilkey-cli exec <command>       Resolve VK: hashes and run"
echo "  veilkey-cli resolve <VK:hash>    Resolve VK hash to value"
if [[ "$OS" != "windows" ]]; then
    echo "  vk_wrap                           Protected shell environment"
    echo "  vk                                Manual VK registration"
fi
