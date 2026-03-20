#!/bin/bash
set -euo pipefail

# VeilKey macOS: Move KEK password file to RAM disk (tmpfs equivalent)
# Usage: bash install/macos/vaultcenter/setup-tmpfs-password.sh
#
# Creates a 1MB RAM disk, copies password file there, updates launchd plist.
# Password exists only in RAM — gone on reboot.
# After reboot, manually copy password back or unlock via POST /api/unlock.

RAMDISK_SIZE_SECTORS=2048  # 1MB (512 bytes per sector)
RAMDISK_MOUNT="/Volumes/veilkey-secrets"
PLIST="$HOME/Library/LaunchAgents/net.veilkey.keycenter.plist"

# Services to handle
SERVICES=(
    "net.veilkey.keycenter:/usr/local/etc/veilkey/keycenter.password"
    "net.veilkey.localvault:/usr/local/etc/veilkey/localvault.password"
)

echo "=== VeilKey Password → RAM Disk ==="
echo ""

# 1. Create RAM disk if not mounted
if mount | grep -q "$RAMDISK_MOUNT"; then
    echo "[1/4] RAM disk already mounted: $RAMDISK_MOUNT"
else
    echo "[1/4] Creating RAM disk..."
    DISK=$(hdiutil attach -nomount ram://$RAMDISK_SIZE_SECTORS)
    DISK=$(echo "$DISK" | tr -d ' ')
    newfs_hfs -v "veilkey-secrets" "$DISK" >/dev/null 2>&1
    mkdir -p "$RAMDISK_MOUNT"
    mount -t hfs "$DISK" "$RAMDISK_MOUNT"
    chmod 700 "$RAMDISK_MOUNT"
    echo "  Created: $RAMDISK_MOUNT ($RAMDISK_SIZE_SECTORS sectors)"
fi

# 2. Copy password files to RAM disk
echo "[2/4] Copying password files to RAM disk..."
for entry in "${SERVICES[@]}"; do
    SERVICE="${entry%%:*}"
    DISK_PW="${entry##*:}"
    RAM_PW="$RAMDISK_MOUNT/$(basename "$DISK_PW")"

    if [ ! -f "$DISK_PW" ]; then
        echo "  $DISK_PW not found, skipping"
        continue
    fi

    cp "$DISK_PW" "$RAM_PW"
    chmod 600 "$RAM_PW"
    echo "  $DISK_PW → $RAM_PW"
done

# 3. Update launchd plists to point to RAM disk
echo "[3/4] Updating launchd plists..."
for entry in "${SERVICES[@]}"; do
    SERVICE="${entry%%:*}"
    DISK_PW="${entry##*:}"
    RAM_PW="$RAMDISK_MOUNT/$(basename "$DISK_PW")"
    PLIST_FILE="$HOME/Library/LaunchAgents/${SERVICE}.plist"

    if [ ! -f "$PLIST_FILE" ]; then
        echo "  $PLIST_FILE not found, skipping"
        continue
    fi

    if grep -q "$DISK_PW" "$PLIST_FILE"; then
        sed -i '' "s|$DISK_PW|$RAM_PW|g" "$PLIST_FILE"
        launchctl unload "$PLIST_FILE" 2>/dev/null || true
        launchctl load "$PLIST_FILE"
        launchctl start "$SERVICE"
        echo "  $SERVICE → $RAM_PW"
    else
        echo "  $SERVICE: already using RAM disk or no password file configured"
    fi
done

# 4. Securely delete disk files
echo "[4/4] Removing plaintext files from disk..."
for entry in "${SERVICES[@]}"; do
    DISK_PW="${entry##*:}"
    if [ -f "$DISK_PW" ]; then
        rm -P "$DISK_PW" 2>/dev/null || rm "$DISK_PW"
        echo "  Deleted: $DISK_PW"
    fi
done

echo ""
echo "=== 완료 ==="
echo ""
echo "  RAM disk: $RAMDISK_MOUNT"
echo "  비밀번호는 메모리에만 존재합니다."
echo ""
echo "  ⚠️  재부팅 시 RAM disk가 사라집니다."
echo "  재부팅 후 수동 unlock 또는 이 스크립트를 다시 실행하세요:"
echo "    bash install/macos/vaultcenter/setup-tmpfs-password.sh"
echo ""
echo "  원복하려면:"
echo "    diskutil eject '$RAMDISK_MOUNT'"
echo ""
