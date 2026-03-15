#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 --self-test | --check <visibility> <registry_access>" >&2
}

assert_policy() {
  local visibility="${1:-}"
  local registry="${2:-}"

  if [[ -z "$visibility" || -z "$registry" ]]; then
    echo "missing visibility or registry access level" >&2
    return 1
  fi

  if [[ "$visibility" == "public" && "$registry" != "private" ]]; then
    echo "public projects must keep container registry private" >&2
    echo "visibility=$visibility registry=$registry" >&2
    return 1
  fi
}

run_self_test() {
  assert_policy public private
  assert_policy internal enabled
  assert_policy private enabled

  if assert_policy public enabled >/dev/null 2>&1; then
    echo "expected public/enabled to fail" >&2
    exit 1
  fi

  if assert_policy public "" >/dev/null 2>&1; then
    echo "expected incomplete policy input to fail" >&2
    exit 1
  fi
}

case "${1:-}" in
  --self-test)
    run_self_test
    ;;
  --check)
    shift
    if [[ $# -ne 2 ]]; then
      usage
      exit 1
    fi
    assert_policy "$1" "$2"
    ;;
  *)
    usage
    exit 1
    ;;
esac
