#!/usr/bin/env bash
set -euo pipefail

profile="${1:-default}"
vmid="${2:-100208}"
lines="${3:-10}"
[[ "$lines" =~ ^[0-9]+$ ]] || { echo "lines must be a positive integer" >&2; exit 2; }

access="/var/log/veilkey-proxy/${profile}.jsonl"
rewrite="/var/log/veilkey-proxy/${profile}-rewrite.jsonl"

echo "== profile =="
echo "vmid=$vmid"
echo "profile=$profile"
echo "access=$access"
echo "rewrite=$rewrite"

echo
echo "== latest access =="
vibe_lxc_ops "$vmid" "test -f '$access' && tail -n '$lines' '$access' || echo missing"

echo
echo "== latest rewrite =="
vibe_lxc_ops "$vmid" "test -f '$rewrite' && tail -n '$lines' '$rewrite' || echo missing"

echo
echo "== access summary =="
vibe_lxc_ops "$vmid" "python3 - <<'PY'
import json
from collections import Counter
path = '$access'
counts = Counter()
errors = Counter()
try:
    with open(path, 'r', encoding='utf-8') as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            obj = json.loads(line)
            counts[obj.get('action', 'unknown')] += 1
            err = obj.get('error')
            if err:
                errors[err] += 1
except FileNotFoundError:
    print('missing')
    raise SystemExit(0)
print('actions=', dict(counts))
if errors:
    print('errors=', dict(errors))
PY"

echo
echo "== rewrite summary =="
vibe_lxc_ops "$vmid" "python3 - <<'PY'
import json
from collections import Counter
path = '$rewrite'
patterns = Counter()
refs = Counter()
try:
    with open(path, 'r', encoding='utf-8') as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            obj = json.loads(line)
            patterns[obj.get('pattern', 'unknown')] += 1
            refs[obj.get('veilkey', 'unknown')] += 1
except FileNotFoundError:
    print('missing')
    raise SystemExit(0)
print('patterns=', dict(patterns))
print('recent_refs=', list(refs.keys())[-5:])
PY"
