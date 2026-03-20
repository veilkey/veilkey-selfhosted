#!/bin/bash
set -euo pipefail

# Smoke test runner
# Runs specified test suites and saves results to tests/smoke/results/
#
# Usage:
#   bash tests/smoke/run.sh <suite> [suite...]
#
# Suites:
#   veil-cli       — install/common/install-veil-cli.sh (requires VEILKEY_URL, VEILKEY_TEST_REF, VEILKEY_TEST_VALUE)
#   localvault     — install/proxmox-lxc-debian/install-localvault.sh (requires VEILKEY_CENTER_URL, VEILKEY_PASSWORD)
#   all            — run all suites
#
# Examples:
#   VEILKEY_URL=https://10.50.0.110:11181 \
#   VEILKEY_TEST_REF=VK:LOCAL:07c52335 \
#   VEILKEY_TEST_VALUE=my-super-secret-value \
#     bash tests/smoke/run.sh veil-cli
#
#   VEILKEY_CENTER_URL=https://10.50.0.110:11181 \
#   VEILKEY_PASSWORD=xxx \
#     bash tests/smoke/run.sh localvault

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RESULTS_DIR="$SCRIPT_DIR/results"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
HOSTNAME="$(hostname)"

mkdir -p "$RESULTS_DIR"

if ! command -v bats &>/dev/null; then
    echo "ERROR: bats not found. Install: apt install bats"
    exit 1
fi

SUITES=("${@:-all}")

run_suite() {
    local name="$1"
    local file="$2"
    local result_file="$RESULTS_DIR/${HOSTNAME}-${name}-${TIMESTAMP}.tap"

    echo "=== Running: $name ==="
    echo ""

    if bats --tap "$file" 2>&1 | tee "$result_file"; then
        echo ""
        echo "PASSED: $name"
    else
        echo ""
        echo "FAILED: $name (see $result_file)"
    fi
    echo "Result: $result_file"
    echo ""
}

for suite in "${SUITES[@]}"; do
    case "$suite" in
        veil-cli)
            run_suite "veil-cli" "$SCRIPT_DIR/common-veil-cli.bats"
            ;;
        localvault)
            run_suite "localvault" "$SCRIPT_DIR/proxmox-lxc-debian-localvault.bats"
            ;;
        all)
            if [[ -n "${VEILKEY_URL:-}" ]]; then
                run_suite "veil-cli" "$SCRIPT_DIR/common-veil-cli.bats"
            else
                echo "SKIP: veil-cli (VEILKEY_URL not set)"
            fi
            if [[ -n "${VEILKEY_CENTER_URL:-}" ]]; then
                run_suite "localvault" "$SCRIPT_DIR/proxmox-lxc-debian-localvault.bats"
            else
                echo "SKIP: localvault (VEILKEY_CENTER_URL not set)"
            fi
            ;;
        *)
            echo "Unknown suite: $suite"
            echo "Available: veil-cli, localvault, all"
            exit 1
            ;;
    esac
done
