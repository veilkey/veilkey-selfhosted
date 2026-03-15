#!/usr/bin/env bash
set -euo pipefail

# VeilKey KeyCenter Installer — macOS
#
# Usage:
#   curl -sf http://your-gitlab/veilkey/veilkey-keycenter/-/raw/main/install/server-macos.sh | bash
#
# Environment:
#   VEILKEY_REPO_URL      Git repo URL (required)
#   VEILKEY_ADDR          Listen address (default: :10180)
#   VEILKEY_TRUSTED_IPS   Comma-separated trusted IPs/CIDRs
#   VEILKEY_INSTALL_DIR   Install path (default: /usr/local/veilkey)
#   VEILKEY_BRANCH        Branch (default: main)
#   GO_VERSION            Go version (default: 1.24.4)
#   VEILKEY_PASSWORD_FILE  Path to file containing KEK password (if set, skips interactive prompt)

INSTALL_DIR="${VEILKEY_INSTALL_DIR:-/usr/local/veilkey}"
BRANCH="${VEILKEY_BRANCH:-main}"
GO_VERSION="${GO_VERSION:-1.24.4}"
DATA_DIR="${INSTALL_DIR}/data"
SERVICE_LABEL="com.veilkey.keycenter"
ADDR="${VEILKEY_ADDR:-:10180}"

echo "=== VeilKey KeyCenter Install (macOS) ==="
echo ""

# --- Install dependencies ---
install_deps() {
    if ! command -v brew &>/dev/null; then
        echo "ERROR: Homebrew required. Install: https://brew.sh" >&2
        exit 1
    fi
    command -v git &>/dev/null || brew install git
}

install_go() {
    if command -v go &>/dev/null; then
        echo "Go: $(go version | awk '{print $3}')"
        return
    fi
    echo "Installing Go ${GO_VERSION}..."
    local arch
    case "$(uname -m)" in
        x86_64) arch="amd64" ;;
        arm64)  arch="arm64" ;;
        *) echo "ERROR: Unsupported arch: $(uname -m)" >&2; exit 1 ;;
    esac
    local tarball="go${GO_VERSION}.darwin-${arch}.tar.gz"
    curl -sfL "https://go.dev/dl/${tarball}" -o "/tmp/${tarball}"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"
    export PATH="/usr/local/go/bin:$PATH"
    echo "  Go $(go version | awk '{print $3}') installed"
}

# --- Clone/update repo ---
clone_repo() {
    REPO_URL="${VEILKEY_REPO_URL:-}"
    if [[ -z "$REPO_URL" ]]; then
        read -rp "Git repo URL: " REPO_URL
        [[ -z "$REPO_URL" ]] && { echo "ERROR: required" >&2; exit 1; }
    fi

    if [[ -d "$INSTALL_DIR/.git" ]]; then
        echo "Updating repo..."
        cd "$INSTALL_DIR"
        git checkout . 2>/dev/null || true
        git fetch origin
        git checkout "$BRANCH"
        git reset --hard "origin/$BRANCH"
    else
        echo "Cloning repo..."
        sudo mkdir -p "$INSTALL_DIR"
        sudo chown "$(whoami)" "$INSTALL_DIR"
        git clone --depth 1 -b "$BRANCH" "${REPO_URL%.git}.git" "$INSTALL_DIR"
    fi
}

# --- Build server ---
build_server() {
    echo "Building veilkey-keycenter..."
    cd "$INSTALL_DIR"
    mkdir -p "${INSTALL_DIR}/bin"
    CGO_ENABLED=1 go build -ldflags="-s -w" -o "${INSTALL_DIR}/bin/veilkey-keycenter" ./cmd/main.go
    echo "  Built: ${INSTALL_DIR}/bin/veilkey-keycenter"
}

# --- Initialize with password ---
setup_init() {
    mkdir -p "$DATA_DIR"
    chmod 700 "$DATA_DIR"

    local salt_file="${DATA_DIR}/salt"

    if [[ -f "$salt_file" ]]; then
        echo "Already initialized (salt file exists). Skipping init."
        return
    fi

    echo ""
    echo "=== KEK Password Setup ==="
    echo "  This password derives the master encryption key (KEK)."
    echo "  Lost password = unrecoverable data."
    echo ""

    local password=""
    if [[ -n "${VEILKEY_PASSWORD_FILE:-}" && -f "${VEILKEY_PASSWORD_FILE}" ]]; then
        password="$(cat "${VEILKEY_PASSWORD_FILE}")"
    fi
    if [[ -z "$password" ]]; then
        read -rsp "Enter KEK password (min 8 chars): " password </dev/tty
        echo ""
        read -rsp "Confirm KEK password: " password2 </dev/tty
        echo ""
        if [[ "$password" != "$password2" ]]; then
            echo "ERROR: Passwords do not match." >&2
            exit 1
        fi
    fi

    if [[ ${#password} -lt 8 ]]; then
        echo "ERROR: Password must be at least 8 characters." >&2
        exit 1
    fi

    # Write password to restricted file for auto-unlock
    local pw_file="${DATA_DIR}/password"
    printf '%s' "$password" > "$pw_file"
    chmod 600 "$pw_file"

    echo "$password" | VEILKEY_DB_PATH="${DATA_DIR}/veilkey.db" "${INSTALL_DIR}/bin/veilkey-keycenter" --init
    echo ""
}

# --- Setup launchd service ---
setup_launchd() {
    local trusted_ips="${VEILKEY_TRUSTED_IPS:-10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1}"
    local password_env=""
    local pw_file="${DATA_DIR}/password"
    if [[ -f "$pw_file" ]]; then
        password_env="<key>VEILKEY_PASSWORD_FILE</key><string>${pw_file}</string>"
    fi

    local plist="/Library/LaunchDaemons/${SERVICE_LABEL}.plist"
    sudo tee "$plist" > /dev/null <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${SERVICE_LABEL}</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/bin/veilkey-keycenter</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>VEILKEY_ADDR</key>
        <string>${ADDR}</string>
        <key>VEILKEY_DB_PATH</key>
        <string>${DATA_DIR}/veilkey.db</string>
        <key>VEILKEY_TRUSTED_IPS</key>
        <string>${trusted_ips}</string>
        ${password_env}
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/veilkey-keycenter.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/veilkey-keycenter.log</string>
</dict>
</plist>
EOF

    sudo launchctl bootout system "${SERVICE_LABEL}" 2>/dev/null || true
    sudo launchctl bootstrap system "$plist"
    echo "  Service: ${SERVICE_LABEL} (launchd)"
}

# --- Verify ---
verify() {
    sleep 2
    local port="${ADDR#:}"
    local url="http://127.0.0.1:${port}/health"
    echo ""
    echo "Verifying..."
    local resp
    resp=$(curl -sf "$url" 2>/dev/null || echo '{}')
    if echo "$resp" | grep -q '"locked"'; then
        echo "  Health check: OK (locked — needs unlock)"
        echo ""
        echo "  Unlock: curl -X POST http://127.0.0.1:${port}/api/unlock \\"
        echo "    -H 'Content-Type: application/json' \\"
        echo "    -d '{\"password\":\"your-password\"}'"
    elif echo "$resp" | grep -q '"ok"'; then
        echo "  Health check: OK (unlocked)"
    else
        echo "  Health check: FAILED"
        echo "  Check: cat /tmp/veilkey-keycenter.log"
        exit 1
    fi
}

# --- Run ---
install_deps
install_go
clone_repo
build_server
setup_init
setup_launchd
verify

echo ""
echo "=== VeilKey KeyCenter installed ==="
echo ""
echo "  Binary:    ${INSTALL_DIR}/bin/veilkey-keycenter"
echo "  Data:      ${DATA_DIR}/"
echo "  Salt:      ${DATA_DIR}/salt"
echo "  Logs:      /tmp/veilkey-keycenter.log"
echo "  Service:   sudo launchctl list | grep veilkey"
echo "  Console:   http://127.0.0.1${ADDR}/"
echo ""
echo "  IMPORTANT: Remember your KEK password. Lost = unrecoverable."
echo ""
