#!/bin/sh
set -eu

PACKAGE=${1:?package path is required}
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT INT TERM

dpkg-deb --info "$PACKAGE" >/dev/null
dpkg-deb --extract "$PACKAGE" "$WORK/root"
dpkg-deb --control "$PACKAGE" "$WORK/control"

test -x "$WORK/root/usr/bin/zfs-pay"
test -x "$WORK/root/usr/lib/zfs-pay/zfs-pay-zed"
test -f "$WORK/root/usr/lib/systemd/system/zfs-pay-reconcile.service"
test -f "$WORK/root/etc/default/zfs-pay"
test -f "$WORK/root/usr/share/doc/zfs-pay/copyright"
test -x "$WORK/control/postinst"
test -x "$WORK/control/prerm"
test -x "$WORK/control/postrm"

for hook in statechange pool_import vdev_attach vdev_clear; do
	test -L "$WORK/root/etc/zfs/zed.d/${hook}-zfs-pay.sh"
	test "$(readlink "$WORK/root/etc/zfs/zed.d/${hook}-zfs-pay.sh")" = "/usr/lib/zfs-pay/zed.d/${hook}-zfs-pay.sh"
done

if find "$WORK/root" -type f \( -iname '*storcli*' -o -iname '*perccli*' \) | grep -q .; then
	echo "proprietary controller binary found in package" >&2
	exit 1
fi

case "$(dpkg-deb --field "$PACKAGE" Architecture)" in
	amd64) file "$WORK/root/usr/bin/zfs-pay" | grep -q 'x86-64' ;;
	arm64) file "$WORK/root/usr/bin/zfs-pay" | grep -Eq 'ARM aarch64|aarch64' ;;
	*) echo "unexpected package architecture" >&2; exit 1 ;;
esac
