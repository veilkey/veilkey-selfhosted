#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

cd "${repo_root}"

bash docs/tools/validate.sh
bash docs/tools/check-hardcoded-values.sh
bash docs/tools/check-generated.sh

echo "Docs doctor passed."
