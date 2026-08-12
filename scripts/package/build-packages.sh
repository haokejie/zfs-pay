#!/bin/sh
set -eu

VERSION=${VERSION:-0.1.0~dev}
PLATFORMS=${PLATFORMS:-linux/amd64,linux/arm64}
OUT_DIR=${OUT_DIR:-dist/packages}
SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-0}
BUILD_DATE=${BUILD_DATE:-1970-01-01T00:00:00Z}
COMMIT=${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || printf unknown)}
RAW="$OUT_DIR/.buildx"

rm -rf "$RAW"
mkdir -p "$RAW" "$OUT_DIR"
docker buildx build \
	--platform "$PLATFORMS" \
	--target artifact \
	--build-arg "VERSION=$VERSION" \
	--build-arg "COMMIT=$COMMIT" \
	--build-arg "BUILD_DATE=$BUILD_DATE" \
	--build-arg "SOURCE_DATE_EPOCH=$SOURCE_DATE_EPOCH" \
	--output "type=local,dest=$RAW" \
	-f packaging/docker/Dockerfile .

find "$RAW" -type f -name 'zfs-pay_*.deb' -exec cp {} "$OUT_DIR/" \;
rm -rf "$RAW"
if command -v sha256sum >/dev/null 2>&1; then
	(cd "$OUT_DIR" && sha256sum zfs-pay_*.deb >SHA256SUMS)
else
	(cd "$OUT_DIR" && shasum -a 256 zfs-pay_*.deb >SHA256SUMS)
fi
