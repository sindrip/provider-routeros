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
# CHR_UDP_PORT forwards a host UDP port to the guest, which qemu's user
# networking does not do by default — without it nothing outside can reach a
# listening service on the CHR, so a WireGuard peer never completes a
# handshake and every field that only moves on an established tunnel
# (last-handshake, rx, tx) stays frozen at its initial value. Set it to give a
# real client a way in:
#
#   CHR_UDP_PORT=13231 hack/chr/run.sh          # host 13231 -> guest 13231
#   CHR_UDP_PORT=13232 CHR_UDP_GUEST_PORT=13231 hack/chr/run.sh
#
# The second form is what two concurrent CHRs need: the guest port is fixed by
# the router's listen-port, so the host ports have to differ.
#
# On arm64 hosts this boots MikroTik's arm64 CHR image under TCG with
# -cpu cortex-a72 (REST in ~25s); everywhere else, the x86-64 image under
# whatever qemu-system-x86_64 does natively (TCG on Apple Silicon: a minute
# or two). Set CHR_ARCH=x86_64 to force the x86 image on an arm64 host.
#
# The arm64 recipe is deliberate and fragile — credit to tikoci/mikropkl
# (Lab/qemu-arm64/NOTES.md) for the analysis:
#   - HVF with -cpu host does NOT work: RouterOS init cannot build its /ram
#     capability files from Apple core IDs and the boot dies or comes up
#     mute. cortex-a72 under TCG is the working pairing, and same-width TCG
#     still beats cross-arch TCG by ~4x.
#   - Firmware must be pflash CODE + writable VARS (copied pristine per
#     boot, like the disk); plain -bios panics "No working init found".
#   - The guest self-reports "damaged system package: bad image" on
#     /system/check-installation — capability files are unpopulatable under
#     qemu. REST CRUD is unaffected (the live suite passes), but if a probe
#     ever behaves oddly, rerun with CHR_ARCH=x86_64 before trusting it.
set -euo pipefail

VERSION="${CHR_VERSION:-7.23.2}"
PORT="${CHR_REST_PORT:-18080}"

case "${CHR_ARCH:-$(uname -m)}" in
arm64 | aarch64)
    SUFFIX="-arm64"
    QEMU=qemu-system-aarch64
    ;;
*)
    SUFFIX=""
    QEMU=qemu-system-x86_64
    ;;
esac

UDP_PORT="${CHR_UDP_PORT:-}"
UDP_GUEST_PORT="${CHR_UDP_GUEST_PORT:-$UDP_PORT}"
# A loopback-bound forward is reachable from the host but not from another
# CHR: qemu's slirp sends a guest's traffic for 10.0.2.2 from the host's own
# address, not from 127.0.0.1, so it never matches. Bind 0.0.0.0 to let two
# CHRs peer with each other.
UDP_BIND="${CHR_UDP_BIND:-127.0.0.1}"
# qemu user networking forwards nothing inbound unless asked; empty when
# CHR_UDP_PORT is unset, so the command line is unchanged by default.
UDPFWD=""
if [ -n "$UDP_PORT" ]; then
    UDPFWD=",hostfwd=udp:$UDP_BIND:$UDP_PORT-:$UDP_GUEST_PORT"
fi

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$ROOT/.work/chr"
CACHE_IMG="$WORK/chr-$VERSION$SUFFIX.img"
RUN_IMG="$WORK/run-$PORT.img"
PIDFILE="$WORK/chr-$PORT.pid"

alive() { [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; }

case "${1:-start}" in
stop)
    if alive; then
        # Read the pid before kill: qemu removes its own pidfile on exit.
        PID="$(cat "$PIDFILE")"
        kill "$PID"
        echo "stopped CHR (pid $PID)"
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
    echo "downloading CHR $VERSION$SUFFIX image..."
    curl -fL --progress-bar "https://download.mikrotik.com/routeros/$VERSION/chr-$VERSION$SUFFIX.img.zip" -o "$CACHE_IMG.zip"
    unzip -p "$CACHE_IMG.zip" >"$CACHE_IMG.tmp"
    mv "$CACHE_IMG.tmp" "$CACHE_IMG"
    rm -f "$CACHE_IMG.zip"
fi

cp "$CACHE_IMG" "$RUN_IMG"

if [ -n "$SUFFIX" ]; then
    SHARE="$(cd "$(dirname "$(command -v "$QEMU")")/../share/qemu" && pwd)"
    cp "$SHARE/edk2-arm-vars.fd" "$WORK/vars-$PORT.fd"
    "$QEMU" \
        -machine virt -cpu cortex-a72 -m 1024 -smp 2 \
        -drive if=pflash,format=raw,readonly=on,unit=0,file="$SHARE/edk2-aarch64-code.fd" \
        -drive if=pflash,format=raw,unit=1,file="$WORK/vars-$PORT.fd" \
        -drive file="$RUN_IMG",format=raw,if=none,id=hd0 \
        -device virtio-blk-pci,drive=hd0 \
        -netdev user,id=n0,hostfwd=tcp:127.0.0.1:$PORT-:80$UDPFWD \
        -device virtio-net-pci,netdev=n0 \
        -netdev user,id=n1 -device virtio-net-pci,netdev=n1 \
        -netdev user,id=n2 -device virtio-net-pci,netdev=n2 \
        -display none -serial none -monitor none \
        -pidfile "$PIDFILE" -daemonize
else
    "$QEMU" \
        -machine pc -m 512 -smp 2 \
        -drive file="$RUN_IMG",format=raw,if=virtio \
        -netdev user,id=n0,hostfwd=tcp:127.0.0.1:$PORT-:80$UDPFWD \
        -device virtio-net-pci,netdev=n0 \
        -netdev user,id=n1 -device virtio-net-pci,netdev=n1 \
        -netdev user,id=n2 -device virtio-net-pci,netdev=n2 \
        -display none -serial none -monitor none \
        -pidfile "$PIDFILE" -daemonize
fi

echo "booting CHR $VERSION (pid $(cat "$PIDFILE")), waiting for REST on port $PORT..."
# Require several consecutive successes: the first requests right after the
# service comes up are sometimes reset or silently dropped.
STREAK=0
for i in $(seq 1 120); do
    if curl -sf -m 2 -u admin: "http://127.0.0.1:$PORT/rest/system/resource" >/dev/null 2>&1; then
        STREAK=$((STREAK + 1))
        if [ "$STREAK" -ge 3 ]; then
            echo "ready: http://127.0.0.1:$PORT/rest (admin, empty password)"
            exit 0
        fi
        sleep 2
        continue
    fi
    STREAK=0
    if ! alive; then
        echo "qemu exited during boot" >&2
        exit 1
    fi
    sleep 5
done

echo "timed out waiting for REST; VM left running (pid $(cat "$PIDFILE"))" >&2
exit 1
