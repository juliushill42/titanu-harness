#!/usr/bin/env bash
set -Eeuo pipefail
umask 022

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || printf 'dev')}"
OUT="${OUT:-$ROOT/dist}"
PKG="titanu-harness-$VERSION"
rm -rf "$OUT"
mkdir -p "$OUT"

command -v go >/dev/null || { echo "Go 1.22+ is required to build release packages." >&2; exit 2; }
go test ./... -count=1
go vet ./...
node --check live.js

package_one() {
  local goos="$1" goarch="$2" label="$3"
  local dir="$OUT/$PKG-$label"
  mkdir -p "$dir"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="-s -w" -o "$dir/titanu-harness" ./cmd/harness
  cp README.md "$dir/README.md"
  cat > "$dir/run.sh" <<'RUN'
#!/usr/bin/env sh
set -eu
HERE="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
export TITANU_HARNESS_ROOT="$HERE"
exec "$HERE/titanu-harness"
RUN
  chmod 0755 "$dir/run.sh" "$dir/titanu-harness"
  (cd "$OUT" && tar -czf "$PKG-$label.tar.gz" "$PKG-$label")
  rm -rf "$dir"
}

package_one linux amd64 linux-amd64
package_one linux arm64 linux-arm64
package_one android arm64 android-arm64-termux

(
  cd "$OUT"
  sha256sum "$PKG-"*.tar.gz > SHA256SUMS
)
printf 'Release packages created in %s\n' "$OUT"
