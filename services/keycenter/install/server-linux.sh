#!/usr/bin/env bash
set -euo pipefail

# VeilKey KeyCenter Installer — Linux
#
# Usage:
#   curl -sf http://your-gitlab/veilkey/veilkey-keycenter/-/raw/main/install/server-linux.sh | bash
#
# Environment:
#   VEILKEY_REPO_URL      Git repo URL (required)
#   VEILKEY_ADDR          Listen address (default: :10180)
#   VEILKEY_TRUSTED_IPS   Comma-separated trusted IPs/CIDRs
#   VEILKEY_INSTALL_DIR   Install path (default: /opt/veilkey)
#   VEILKEY_BRANCH        Branch (default: main)
#   GO_VERSION            Go version (default: 1.24.4)
#   VEILKEY_PASSWORD_FILE  Path to file containing KEK password (if set, skips interactive prompt)

INSTALL_DIR="${VEILKEY_INSTALL_DIR:-/opt/veilkey}"
BRANCH="${VEILKEY_BRANCH:-main}"
GO_VERSION="${GO_VERSION:-1.24.4}"
DATA_DIR="${INSTALL_DIR}/data"
SERVICE_NAME="veilkey-keycenter"
ADDR="${VEILKEY_ADDR:-:10180}"

echo "=== VeilKey KeyCenter Install (Linux) ==="
echo ""

# --- Must be root ---
if [[ $EUID -ne 0 ]]; then
    echo "ERROR: Run as root (sudo)" >&2
    exit 1
fi

# --- Install dependencies ---
install_deps() {
    local missing=()
    command -v git &>/dev/null || missing+=(git)
    command -v gcc &>/dev/null || missing+=(gcc)
    command -v make &>/dev/null || missing+=(make)
    command -v curl &>/dev/null || missing+=(curl)

    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "Installing: ${missing[*]}"
        if command -v apt-get &>/dev/null; then
            apt-get update -qq && apt-get install -y -qq "${missing[@]}" build-essential
        elif command -v yum &>/dev/null; then
            yum install -y "${missing[@]}" gcc-c++
        elif command -v apk &>/dev/null; then
            apk add "${missing[@]}" build-base
        else
            echo "ERROR: Install manually: ${missing[*]}" >&2
            exit 1
        fi
    fi
}

install_go() {
    if command -v go &>/dev/null; then
        echo "Go: $(go version | awk '{print $3}')"
        return
    fi
    echo "Installing Go ${GO_VERSION}..."
    local arch
    case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *) echo "ERROR: Unsupported arch: $(uname -m)" >&2; exit 1 ;;
    esac
    local tarball="go${GO_VERSION}.linux-${arch}.tar.gz"
    curl -sfL "https://go.dev/dl/${tarball}" -o "/tmp/${tarball}"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"
    export PATH="/usr/local/go/bin:$PATH"
    [[ -d /etc/profile.d ]] && echo 'export PATH="/usr/local/go/bin:$PATH"' > /etc/profile.d/go.sh
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
        [[ -d "$INSTALL_DIR" ]] || mkdir -p "$INSTALL_DIR"
        git clone --depth 1 -b "$BRANCH" "${REPO_URL%.git}.git" "$INSTALL_DIR"
    fi
}

# --- Build server ---
build_server() {
    echo "Building veilkey-keycenter..."
    cd "$INSTALL_DIR"
    CGO_ENABLED=1 go build -ldflags="-s -w" -o "/usr/local/bin/${SERVICE_NAME}" ./cmd/main.go
    chmod +x "/usr/local/bin/${SERVICE_NAME}"
    echo "  Built: /usr/local/bin/${SERVICE_NAME}"
}

# --- Build scope-env-sync ---
build_scope_env_sync() {
    if [[ -d "$INSTALL_DIR/server/cmd/scope-env-sync" ]]; then
        echo "Building scope-env-sync..."
        cd "$INSTALL_DIR"
        CGO_ENABLED=0 go build -ldflags="-s -w" -o "/usr/local/bin/scope-env-sync" ./cmd/scope-env-sync/
        chmod +x "/usr/local/bin/scope-env-sync"
        echo "  Built: /usr/local/bin/scope-env-sync"
    fi
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
    local pw_file="${VEILKEY_PASSWORD_FILE:-}"
    if [[ -n "$pw_file" && -f "$pw_file" ]]; then
        password="$(cat "$pw_file")"
    else
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

    # Write password to a restricted file for auto-unlock (never pass via env)
    local runtime_pw_file="${DATA_DIR}/password"
    printf '%s' "$password" > "$runtime_pw_file"
    chmod 600 "$runtime_pw_file"

    # Run server --init with password on stdin
    echo "$password" | /usr/local/bin/${SERVICE_NAME} --init
    echo ""
}

# --- Setup systemd service ---
setup_systemd() {
    local trusted_ips="${VEILKEY_TRUSTED_IPS:-}"
    if [[ -z "$trusted_ips" ]]; then
        trusted_ips="10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1"
    fi

    # Use password file for auto-unlock (never store password in env vars)
    local env_file_line=""
    local password_env=""
    local pw_file="${DATA_DIR}/password"
    if [[ -f "$pw_file" ]]; then
        env_file_line="Environment=VEILKEY_PASSWORD_FILE=${pw_file}"
        password_env="# Auto-unlock enabled via VEILKEY_PASSWORD_FILE=${pw_file}"
    else
        password_env="# Manual unlock required: POST /api/unlock {\"password\":\"...\"}"
    fi

    cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=VeilKey KeyCenter
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/${SERVICE_NAME}
Environment=VEILKEY_ADDR=${ADDR}
Environment=VEILKEY_DB_PATH=${DATA_DIR}/veilkey.db
Environment=VEILKEY_TRUSTED_IPS=${trusted_ips}
${env_file_line}
${password_env}
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable "${SERVICE_NAME}"
    systemctl restart "${SERVICE_NAME}"
    echo "  Service: ${SERVICE_NAME} (enabled + started)"
}

# --- Verify ---
verify() {
    sleep 2
    local port="${ADDR#:}"
    [[ "$port" == "$ADDR" ]] && port="${ADDR##*:}"
    local url="https://127.0.0.1:${port}/health"
    echo ""
    echo "Verifying..."
    local resp
    resp=$(curl -sfk "$url" 2>/dev/null || echo '{}')
    if echo "$resp" | grep -q '"locked"'; then
        echo "  Health check: OK (locked — needs unlock)"
        echo ""
        echo "  Unlock: curl -k -X POST https://127.0.0.1:${port}/api/unlock \\"
        echo "    -H 'Content-Type: application/json' \\"
        echo "    -d '{\"password\":\"your-password\"}'"
    elif echo "$resp" | grep -q '"ok"'; then
        echo "  Health check: OK (unlocked)"
    else
        echo "  Health check: FAILED"
        echo "  Check: journalctl -u ${SERVICE_NAME} -n 20"
        exit 1
    fi
}

# --- Run ---
install_deps
install_go
clone_repo
build_server
build_scope_env_sync
setup_init
setup_systemd
verify

echo ""
echo "=== VeilKey KeyCenter installed ==="
echo ""
echo "  Binary:    /usr/local/bin/${SERVICE_NAME}"
echo "  Data:      ${DATA_DIR}/"
echo "  Salt:      ${DATA_DIR}/salt"
echo "  Service:   systemctl status ${SERVICE_NAME}"
echo "  Logs:      journalctl -u ${SERVICE_NAME} -f"
echo "  Console:   http://$(hostname -I | awk '{print $1}')${ADDR}/"
echo ""
echo "  IMPORTANT: Remember your KEK password. Lost = unrecoverable."
echo ""
