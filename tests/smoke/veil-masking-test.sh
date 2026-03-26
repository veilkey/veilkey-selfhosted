#!/bin/bash
# Smoke test: veil shell masking integration test
# Requires: veilkey-cli deployed, VaultCenter running + unlocked
# Exit 1 on any failure

set -e

VEILKEY_LOCALVAULT_URL="${VEILKEY_LOCALVAULT_URL:-https://vaultcenter.50.internal.kr}"
export VEILKEY_LOCALVAULT_URL VEILKEY_TLS_INSECURE=1

# Check VaultCenter is up
STATUS=$(curl -sk "$VEILKEY_LOCALVAULT_URL/api/status" 2>/dev/null | python3 -c "import json,sys; print(json.load(sys.stdin).get('locked','error'))" 2>/dev/null)
if [ "$STATUS" = "error" ] || [ -z "$STATUS" ]; then
    echo "SKIP: VaultCenter not reachable"
    exit 0
fi
if [ "$STATUS" = "True" ]; then
    echo "SKIP: VaultCenter is locked"
    exit 0
fi

PWFILE=$(mktemp)
echo "Ghdrhkdgh1@" > "$PWFILE"
export VEILKEY_PASSWORD_FILE="$PWFILE"

FAIL=0

# Test 1: Completed line output must use full VK:LOCAL: ref
echo "TEST 1: bash error uses full VK:LOCAL: ref"
OUTPUT=$(python3 -c "
import subprocess, os, time, pty, select, re
master, slave = pty.openpty()
proc = subprocess.Popen(
    ['/usr/local/bin/veilkey-cli', 'wrap-pty', 'bash', '-c', 'echo Ghdrhkdgh1@; exit'],
    stdin=slave, stdout=slave, stderr=slave, close_fds=True, env=os.environ.copy()
)
os.close(slave)
buf = b''
deadline = time.time() + 15
while time.time() < deadline:
    r, _, _ = select.select([master], [], [], 1)
    if r:
        try:
            d = os.read(master, 8192)
            if not d: break
            buf += d
        except: break
    if proc.poll() is not None: break
os.close(master)
clean = re.sub(r'\x1b\[[0-9;]*[a-zA-Z]', '', buf.decode('utf-8', errors='replace'))
# Check for VK:LOCAL: in output (completed line from echo command)
if 'VK:LOCAL:' in clean:
    print('PASS')
else:
    # Check if any VK: ref exists
    vk = re.findall(r'VK:\S+', clean)
    if vk:
        print(f'FAIL:compact:{vk[0]}')
    else:
        print('FAIL:no_ref')
" 2>/dev/null)

case "$OUTPUT" in
    PASS) echo "  PASS: full VK:LOCAL: ref in output" ;;
    FAIL:compact:*) echo "  FAIL: compact ref without LOCAL: ${OUTPUT#FAIL:compact:}"; FAIL=1 ;;
    FAIL:no_ref) echo "  FAIL: no VK ref in output at all"; FAIL=1 ;;
    *) echo "  FAIL: unexpected: $OUTPUT"; FAIL=1 ;;
esac

# Test 2: No VK:LOC fragments on arrow keys
echo "TEST 2: no VK:LOC fragments"
FRAG=$(python3 -c "
import subprocess, os, time, pty, select, re
master, slave = pty.openpty()
proc = subprocess.Popen(
    ['/usr/local/bin/veilkey-cli', 'wrap-pty', 'bash', '--norc', '--noprofile'],
    stdin=slave, stdout=slave, stderr=slave, close_fds=True, env=os.environ.copy()
)
os.close(slave)
def drain(t=2):
    buf = b''
    dl = time.time() + t
    while time.time() < dl:
        r, _, _ = select.select([master], [], [], 0.3)
        if r:
            try: buf += os.read(master, 8192)
            except: break
    return buf
time.sleep(3); drain(2)
os.write(master, b'Ghdrhkdgh1@\n'); time.sleep(1); drain(2)
for _ in range(5):
    os.write(master, b'\x1b[A'); time.sleep(0.05)
for _ in range(3):
    os.write(master, b'\x1b[B'); time.sleep(0.05)
time.sleep(0.5)
buf = drain(2)
os.write(master, b'exit\n'); time.sleep(1)
os.close(master); proc.wait()
clean = re.sub(r'\x1b\[[0-9;]*[a-zA-Z]', '', buf.decode('utf-8', errors='replace'))
if 'VK:LOCVK:' in clean or re.search(r'VK:LOC[^A\s:]', clean):
    print('FAIL')
else:
    print('PASS')
" 2>/dev/null)

if [ "$FRAG" = "PASS" ]; then
    echo "  PASS: no VK:LOC fragments"
else
    echo "  FAIL: VK:LOC fragment detected"; FAIL=1
fi

rm -f "$PWFILE"
exit $FAIL
