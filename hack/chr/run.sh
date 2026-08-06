#!/usr/bin/env bash
# run.sh boots a disposable RouterOS CHR under qemu for hack/uniqprobe and
# hack/inspectdump. The pristine image is cached under .work/chr and copied
# fresh for every boot, so router state never survives a restart. REST is
# forwarded to 127.0.0.1:18080 (admin, empty password); the guest gets three
# NICs so probes that reference ether2/ether3 work.
#
#   hack/chr/run.sh          boot and wait until REST answers
#   hack/chr/run.sh stop     shut the VM down
#   hack/chr/run.sh status   report whether it is running
#
# CHR is x86-64 only, so on Apple Silicon this runs under TCG emulation:
# expect a boot to take a minute or two.
set -euo pipefail

VERSION="${CHR_VERSION:-7.23.2}"
PORT="${CHR_REST_PORT:-18080}"

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$ROOT/.work/chr"
CACHE_IMG="$WORK/chr-$VERSION.img"
RUN_IMG="$WORK/run-$PORT.img"
PIDFILE="$WORK/chr-$PORT.pid"

alive() { [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; }

case "${1:-start}" in
stop)
    if alive; then
        kill "$(cat "$PIDFILE")"
        echo "stopped CHR (pid $(cat "$PIDFILE"))"
    else
        echo "CHR is not running"
    fi
    rm -f "$PIDFILE"
    exit 0
    ;;
status)
    if alive; then
        echo "CHR $VERSION running (pid $(cat "$PIDFILE")), REST at http://127.0.0.1:$PORT/rest"
    else
        echo "CHR is not running"
    fi
    exit 0
    ;;
start) ;;
*)
    echo "usage: $0 [start|stop|status]" >&2
    exit 2
    ;;
esac

if alive; then
    echo "CHR already running (pid $(cat "$PIDFILE")), REST at http://127.0.0.1:$PORT/rest"
    exit 0
fi

mkdir -p "$WORK"
if [ ! -f "$CACHE_IMG" ]; then
    echo "downloading CHR $VERSION image..."
    curl -fL --progress-bar "https://download.mikrotik.com/routeros/$VERSION/chr-$VERSION.img.zip" -o "$CACHE_IMG.zip"
    unzip -p "$CACHE_IMG.zip" >"$CACHE_IMG.tmp"
    mv "$CACHE_IMG.tmp" "$CACHE_IMG"
    rm -f "$CACHE_IMG.zip"
fi

cp "$CACHE_IMG" "$RUN_IMG"

qemu-system-x86_64 \
    -machine pc -m 512 -smp 2 \
    -drive file="$RUN_IMG",format=raw,if=virtio \
    -netdev user,id=n0,hostfwd=tcp:127.0.0.1:$PORT-:80 \
    -device virtio-net-pci,netdev=n0 \
    -netdev user,id=n1 -device virtio-net-pci,netdev=n1 \
    -netdev user,id=n2 -device virtio-net-pci,netdev=n2 \
    -display none -serial none -monitor none \
    -pidfile "$PIDFILE" -daemonize

echo "booting CHR $VERSION (pid $(cat "$PIDFILE")), waiting for REST on port $PORT..."
for i in $(seq 1 120); do
    if curl -sf -m 2 -u admin: "http://127.0.0.1:$PORT/rest/system/resource" >/dev/null 2>&1; then
        echo "ready: http://127.0.0.1:$PORT/rest (admin, empty password)"
        exit 0
    fi
    if ! alive; then
        echo "qemu exited during boot" >&2
        exit 1
    fi
    sleep 5
done

echo "timed out waiting for REST; VM left running (pid $(cat "$PIDFILE"))" >&2
exit 1
