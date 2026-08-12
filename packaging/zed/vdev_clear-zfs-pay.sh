#!/bin/sh
set -eu

if [ -r /etc/default/zfs-pay ]; then
	. /etc/default/zfs-pay
fi

exec /usr/lib/zfs-pay/zfs-pay-zed reconcile
