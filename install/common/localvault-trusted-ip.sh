#!/bin/bash
# Resolve only the VaultCenter address for the standalone installer default.
# Explicit VEILKEY_TRUSTED_IPS values remain an operator-controlled override.
veilkey_center_ip() {
    local host address
    host="${1#*://}"
    host="${host%%/*}"
    if [[ "$host" == \[* ]]; then
        host="${host#\[}"
        host="${host%%\]*}"
    else
        host="${host%%:*}"
    fi
    if command -v getent >/dev/null 2>&1; then
        address=$(getent ahosts "$host" | awk '$1 !~ /^127\./ && $1 != "::1" && $1 != "0.0.0.0" && $1 != "::" { print $1; exit }')
    elif command -v dscacheutil >/dev/null 2>&1; then
        address=$(dscacheutil -q host -a name "$host" | awk '($1 == "ip_address:" || $1 == "ipv6_address:") && $2 !~ /^127\./ && $2 != "::1" && $2 != "0.0.0.0" && $2 != "::" { print $2; exit }')
    else
        echo "ERROR: No supported host resolver; set VEILKEY_TRUSTED_IPS to the exact VaultCenter address." >&2
        return 1
    fi
    if [[ -z "$address" ]]; then
        echo "ERROR: Cannot resolve a non-loopback VaultCenter IP; set VEILKEY_TRUSTED_IPS to its exact address." >&2
        return 1
    fi
    printf '%s\n' "$address"
}
