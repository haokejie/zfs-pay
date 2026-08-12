#!/bin/sh
set -eu

OUT_DIR=${OUT_DIR:-dist/package-lifecycle}
OLD_VERSION=${OLD_VERSION:-0.0.0~test1}
NEW_VERSION=${NEW_VERSION:-0.0.0~test2}

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"
VERSION="$OLD_VERSION" PLATFORMS=linux/amd64 OUT_DIR="$OUT_DIR" ./scripts/package/build-packages.sh
rm -f "$OUT_DIR/SHA256SUMS"
VERSION="$NEW_VERSION" PLATFORMS=linux/amd64 OUT_DIR="$OUT_DIR" ./scripts/package/build-packages.sh

OLD_PACKAGE=$(find "$OUT_DIR" -maxdepth 1 -name "zfs-pay_${OLD_VERSION}_amd64.deb" -print -quit)
NEW_PACKAGE=$(find "$OUT_DIR" -maxdepth 1 -name "zfs-pay_${NEW_VERSION}_amd64.deb" -print -quit)
docker build --platform linux/amd64 -t zfs-pay-lifecycle-test -f packaging/docker/Dockerfile.lifecycle .
docker run --rm --platform linux/amd64 \
	-v "$(pwd)/$OUT_DIR:/packages:ro" \
	-v "$(pwd)/scripts/package/container-lifecycle.sh:/test/container-lifecycle.sh:ro" \
	zfs-pay-lifecycle-test sh /test/container-lifecycle.sh \
	"/packages/$(basename "$OLD_PACKAGE")" "/packages/$(basename "$NEW_PACKAGE")"
