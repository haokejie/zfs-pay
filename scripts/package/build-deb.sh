#!/bin/sh
set -eu

: "${ARCH:?ARCH is required}"
: "${VERSION:?VERSION is required}"
: "${BINARY:?BINARY is required}"
: "${HELPER:?HELPER is required}"

case "$ARCH" in
	amd64|arm64) ;;
	*) echo "unsupported Debian architecture: $ARCH" >&2; exit 2 ;;
esac

case "$VERSION" in
	[0-9]*) ;;
	*) echo "Debian version must begin with a digit: $VERSION" >&2; exit 2 ;;
esac

OUT_DIR=${OUT_DIR:-dist/packages}
SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-0}
STAGING=$(mktemp -d)
trap 'rm -rf "$STAGING"' EXIT INT TERM
ROOT="$STAGING/root"

install -d -m 0755 "$ROOT/DEBIAN" "$ROOT/usr/bin" "$ROOT/usr/lib/zfs-pay/zed.d"
install -d -m 0755 "$ROOT/usr/lib/systemd/system" "$ROOT/usr/share/doc/zfs-pay" "$ROOT/etc/default" "$ROOT/etc/zfs/zed.d"
install -d -m 0750 "$ROOT/var/lib/zfs-pay"
install -m 0755 "$BINARY" "$ROOT/usr/bin/zfs-pay"
install -m 0755 "$HELPER" "$ROOT/usr/lib/zfs-pay/zfs-pay-zed"
install -m 0755 packaging/zed/*.sh "$ROOT/usr/lib/zfs-pay/zed.d/"
for hook in "$ROOT"/usr/lib/zfs-pay/zed.d/*.sh; do
	ln -s "/usr/lib/zfs-pay/zed.d/$(basename "$hook")" "$ROOT/etc/zfs/zed.d/$(basename "$hook")"
done
install -m 0644 packaging/systemd/zfs-pay-reconcile.service "$ROOT/usr/lib/systemd/system/"
install -m 0644 packaging/default/zfs-pay "$ROOT/etc/default/zfs-pay"
install -m 0644 LICENSE "$ROOT/usr/share/doc/zfs-pay/copyright"
install -m 0644 README.md docs/install.md "$ROOT/usr/share/doc/zfs-pay/"

sed -e "s/@VERSION@/$VERSION/g" -e "s/@ARCH@/$ARCH/g" packaging/debian/control >"$ROOT/DEBIAN/control"
install -m 0644 packaging/debian/conffiles "$ROOT/DEBIAN/conffiles"
install -m 0755 packaging/debian/postinst packaging/debian/prerm packaging/debian/postrm "$ROOT/DEBIAN/"

find "$ROOT" -exec touch -h -d "@$SOURCE_DATE_EPOCH" {} +
mkdir -p "$OUT_DIR"
PACKAGE="$OUT_DIR/zfs-pay_${VERSION}_${ARCH}.deb"
dpkg-deb --root-owner-group -Zxz --build "$ROOT" "$PACKAGE" >/dev/null
printf '%s\n' "$PACKAGE"
