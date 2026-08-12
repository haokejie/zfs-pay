#!/bin/sh
set -eu

OLD_PACKAGE=${1:?old package is required}
NEW_PACKAGE=${2:?new package is required}

dpkg -i "$OLD_PACKAGE" >/dev/null
test -x /usr/bin/zfs-pay
test -x /usr/lib/zfs-pay/zfs-pay-zed
test -L /etc/zfs/zed.d/statechange-zfs-pay.sh
test -f /usr/lib/systemd/system/zfs-pay-reconcile.service
test -f /etc/default/zfs-pay
test -d /var/lib/zfs-pay
/usr/bin/zfs-pay --version | grep -q "$(dpkg-deb --field "$OLD_PACKAGE" Version)"
systemctl is-enabled zfs-pay-reconcile.service | grep -q enabled
printf '[Unit]\nDescription=Test ZFS import target\n' >/usr/lib/systemd/system/zfs-import.target
systemd-analyze verify /usr/lib/systemd/system/zfs-pay-reconcile.service
set +e
STATUS_OUTPUT=$(/usr/bin/zfs-pay status 2>&1)
STATUS_CODE=$?
set -e
test "$STATUS_CODE" -eq 4
printf '%s\n' "$STATUS_OUTPUT" | grep -q 'command not found'

rm -f /tmp/zpool-calls
cat >/usr/local/bin/zpool <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>/tmp/zpool-calls
exit 99
EOF
chmod 0755 /usr/local/bin/zpool
rm -f /tmp/ledctl-calls
cat >/usr/local/bin/ledctl <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>/tmp/ledctl-calls
EOF
chmod 0755 /usr/local/bin/ledctl

cat >/var/lib/zfs-pay/state.json <<'EOF'
{"schema_version":1,"mappings":{},"managed_leds":{"ledctl:linux/block/sdz":{"pool":"tank","guid":"101","bay":{"backend":"ledctl","controller":"linux","enclosure":"block","slot":"sdz","capability":"locate","led_state":"unknown"},"since":"2026-08-12T00:00:00Z"}},"last_events":{}}
EOF
dpkg -i "$NEW_PACKAGE" >/dev/null
/usr/bin/zfs-pay --version | grep -q "$(dpkg-deb --field "$NEW_PACKAGE" Version)"
test -f /var/lib/zfs-pay/state.json

ZFS_PAY_ENABLE_LEDCTL=1 dpkg --remove zfs-pay >/dev/null
test ! -e /usr/bin/zfs-pay
test ! -e /usr/lib/zfs-pay/zfs-pay-zed
test ! -e /etc/zfs/zed.d/statechange-zfs-pay.sh
test ! -e /usr/lib/systemd/system/zfs-pay-reconcile.service
test ! -e /etc/systemd/system/multi-user.target.wants/zfs-pay-reconcile.service
test -f /etc/default/zfs-pay
test -f /var/lib/zfs-pay/state.json
test "$(cat /tmp/ledctl-calls)" = "locate_off=/dev/sdz"
grep -q '"managed_leds": {}' /var/lib/zfs-pay/state.json

dpkg --purge zfs-pay >/dev/null
test ! -e /etc/default/zfs-pay
test ! -e /var/lib/zfs-pay
test ! -s /tmp/zpool-calls
if [ -d /etc/zfs/zed.d ]; then
	! find /etc/zfs/zed.d -type l -lname '*zfs-pay*' | grep -q .
fi
