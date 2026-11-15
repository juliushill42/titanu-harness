#!/usr/bin/env bash
# One-command TitanU Harness + LittleBit edge backend build, test, launch.
set -Eeuo pipefail
umask 077
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"
if ! command -v go >/dev/null 2>&1; then
  if command -v pkg >/dev/null 2>&1; then pkg install -y golang; else printf 'Go 1.22+ required (https://go.dev/dl).\n' >&2; exit 2; fi
fi
case "${1:-run}" in
  test)
    go test ./... -count=1
    go vet ./...
    ;;
  run|start)
    go test ./... -count=1
    go vet ./...
    mkdir -p bin
    go build -trimpath -o bin/titanu-harness ./cmd/harness
    go build -trimpath -o bin/titanu-littlebit ./cmd/littlebit
    proof="$(./bin/titanu-littlebit apply -artifact testdata/littlebit-layer.json -input 3,5)"
    if [[ "$proof" != '{"output":[-2,2]}' ]]; then
      printf 'LittleBit runtime proof failed: %s\n' "$proof" >&2
      exit 1
    fi
    printf 'LittleBit runtime proof: PASS %s\n' "$proof"
    export TITANU_HARNESS_ROOT="$ROOT"
    exec ./bin/titanu-harness
    ;;
  *) printf 'Usage: bash bootstrap.sh [run|test]\n' >&2; exit 2 ;;
esac
