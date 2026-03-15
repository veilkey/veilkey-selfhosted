#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  [[ "$haystack" == *"$needle"* ]] || fail "expected substring [$needle]"
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  [[ "$haystack" != *"$needle"* ]] || fail "unexpected substring [$needle]"
}

assert_file_contains() {
  local path="$1"
  local needle="$2"
  [[ -f "$path" ]] || fail "missing file $path"
  grep -Fq "$needle" "$path" || fail "expected [$needle] in $path"
}

assert_eq() {
  local got="$1"
  local want="$2"
  [[ "$got" == "$want" ]] || fail "expected [$want], got [$got]"
}
