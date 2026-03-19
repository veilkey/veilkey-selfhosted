#!/bin/bash
set -euo pipefail

# Chain E2E Test — vaultcenter (validator) ↔ localvault (full node)
# Usage: ./scripts/chain-e2e-test.sh

TMPDIR=$(mktemp -d)
VC_DIR="$TMPDIR/vc"
LV_DIR="$TMPDIR/lv"
mkdir -p "$VC_DIR/data" "$LV_DIR/data"

VC_PASS="testpass12345678"
LV_PASS="lvtestpass123456"

echo "$VC_PASS" > "$VC_DIR/pw.txt"
echo "$LV_PASS" > "$LV_DIR/pw.txt"

cleanup() {
  echo "Cleaning up..."
  kill $VC_PID $LV_PID 2>/dev/null || true
  sleep 1
  rm -rf "$TMPDIR"
  echo "Done."
}
trap cleanup EXIT

echo "=== 1. Building binaries ==="
cd "$(dirname "$0")/../services/vaultcenter"
go build -o "$TMPDIR/vaultcenter" ./cmd/main.go
cd "../localvault"
go build -o "$TMPDIR/localvault" ./cmd/main.go
cd ../..

echo "=== 2. Initializing vaultcenter ==="
echo "$VC_PASS" | VEILKEY_DB_PATH="$VC_DIR/data/vc.db" "$TMPDIR/vaultcenter" init --root 2>&1 | grep "Node ID"

echo "=== 3. Initializing localvault ==="
echo "$LV_PASS" | VEILKEY_DB_PATH="$LV_DIR/data/lv.db" "$TMPDIR/localvault" init --root 2>&1 | grep "Node ID"

echo "=== 4. Starting vaultcenter (validator) ==="
VEILKEY_DB_PATH="$VC_DIR/data/vc.db" \
VEILKEY_ADDR=127.0.0.1:10181 \
VEILKEY_PASSWORD_FILE="$VC_DIR/pw.txt" \
VEILKEY_CHAIN_HOME="$VC_DIR/chain" \
VEILKEY_CHAIN_P2P_ADDR=127.0.0.1:26656 \
VEILKEY_TRUSTED_IPS=127.0.0.1 \
"$TMPDIR/vaultcenter" server &
VC_PID=$!
sleep 4

VC_NODE_ID=$(curl -s http://127.0.0.1:26657/status | python3 -c "import sys,json; print(json.load(sys.stdin)['result']['node_info']['id'])")
echo "VC node_id: $VC_NODE_ID"

echo "=== 5. Copying genesis to localvault ==="
mkdir -p "$LV_DIR/chain/config"
cp "$VC_DIR/chain/config/genesis.json" "$LV_DIR/chain/config/"

echo "=== 6. Starting localvault (full node) ==="
VEILKEY_DB_PATH="$LV_DIR/data/lv.db" \
VEILKEY_ADDR=127.0.0.1:10182 \
VEILKEY_PASSWORD_FILE="$LV_DIR/pw.txt" \
VEILKEY_CHAIN_HOME="$LV_DIR/chain" \
VEILKEY_CHAIN_RPC_LISTEN=tcp://127.0.0.1:26667 \
VEILKEY_CHAIN_P2P_LISTEN=tcp://127.0.0.1:26666 \
VEILKEY_CHAIN_PERSISTENT_PEERS="${VC_NODE_ID}@127.0.0.1:26656" \
VEILKEY_VAULTCENTER_URL=http://127.0.0.1:10181 \
"$TMPDIR/localvault" server &
LV_PID=$!
sleep 8

echo "=== 7. Checking P2P sync ==="
VC_HEIGHT=$(curl -s http://127.0.0.1:26657/status | python3 -c "import sys,json; print(json.load(sys.stdin)['result']['sync_info']['latest_block_height'])")
LV_HEIGHT=$(curl -s http://127.0.0.1:26667/status | python3 -c "import sys,json; print(json.load(sys.stdin)['result']['sync_info']['latest_block_height'])")
VC_PEERS=$(curl -s http://127.0.0.1:26657/net_info | python3 -c "import sys,json; print(json.load(sys.stdin)['result']['n_peers'])")
LV_PEERS=$(curl -s http://127.0.0.1:26667/net_info | python3 -c "import sys,json; print(json.load(sys.stdin)['result']['n_peers'])")
LV_CATCHING=$(curl -s http://127.0.0.1:26667/status | python3 -c "import sys,json; print(json.load(sys.stdin)['result']['sync_info']['catching_up'])")

echo "VC: height=$VC_HEIGHT peers=$VC_PEERS"
echo "LV: height=$LV_HEIGHT peers=$LV_PEERS catching_up=$LV_CATCHING"

echo "=== 8. Sending test TX via CometBFT RPC ==="
TX_JSON=$(python3 -c "
import json, uuid, datetime
payload = {'ref_family':'VK','ref_scope':'TEMP','ref_id':'e2etest001','secret_name':'E2E_TEST','agent_hash':'test','ciphertext':'dGVzdA==','version':1,'status':'temp'}
env = {'type':'SaveTokenRef','nonce':str(uuid.uuid4()),'timestamp':datetime.datetime.now(datetime.UTC).isoformat().replace('+00:00','Z'),'actor_type':'test','actor_id':'127.0.0.1','source':'e2e_test','payload':payload}
print(json.dumps(env))
")
TX_HEX=$(echo -n "$TX_JSON" | xxd -p | tr -d '\n')
TX_RESULT=$(curl -s "http://127.0.0.1:26657/broadcast_tx_commit?tx=0x${TX_HEX}" | python3 -c "import sys,json; d=json.load(sys.stdin)['result']; print(f\"check={d['check_tx']['code']} exec={d['tx_result']['code']}\")")
echo "TX result: $TX_RESULT"

sleep 3

echo "=== 9. Verify sync after TX ==="
VC_HEIGHT2=$(curl -s http://127.0.0.1:26657/status | python3 -c "import sys,json; print(json.load(sys.stdin)['result']['sync_info']['latest_block_height'])")
LV_HEIGHT2=$(curl -s http://127.0.0.1:26667/status | python3 -c "import sys,json; print(json.load(sys.stdin)['result']['sync_info']['latest_block_height'])")
echo "VC: height=$VC_HEIGHT2"
echo "LV: height=$LV_HEIGHT2"

if [ "$VC_HEIGHT2" = "$LV_HEIGHT2" ]; then
  echo "✅ PASS — both nodes at same height after TX"
else
  echo "❌ FAIL — heights differ: VC=$VC_HEIGHT2 LV=$LV_HEIGHT2"
  exit 1
fi

echo "=== E2E Test Complete ==="
