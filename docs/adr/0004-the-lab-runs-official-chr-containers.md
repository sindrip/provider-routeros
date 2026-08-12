# The lab runs MikroTik's official CHR containers under docker compose

Since RouterOS 7.22 MikroTik publishes CHR as a multi-arch container image
(`mikrotik/chr`: amd64 + arm64, every release and prerelease channel as a
tag) — qemu around the CHR disk with KVM auto-detection and every container
interface bridged to a VM NIC. The lab uses it as-is, orchestrated by plain
docker compose: one stock dnsmasq sidecar answers the router's own
dhcp-client (the image bridges rather than NATs its management interface,
and a pinned container MAC makes the lease deterministic — the VM's MAC is
the container's plus one), and one stock socat sidecar publishes REST to
localhost for macOS hosts, which cannot reach Docker-unknown addresses.

Rejected: a hand-built qemu-in-container node image (built and booting
before the official image was found — it duplicated what MikroTik now
maintains, and probing the vendor's own artifact is worth more as
evidence); containerlab as orchestrator (its value is VM-wrapping kinds and
veth topologies, but the official image already is a container and ships
its own interface bridging, while containerlab publishes no macOS build and
would keep a shim or a Linux VM in the workflow forever — revisit if a
topology ever needs link impairments or another vendor's NOS); vrnetlab
(x86-only, and its RouterOS kind would need forked arm64 and REST-port
patches).

Consequences: probing a RouterOS version or channel is a tag change
(`CHR_VERSION=7.24rc4`), CI gets KVM speed with the identical compose file,
and the router's serial console remains reachable at `docker attach` for
questions REST cannot answer — but never with a piped stdin, which
terminates qemu on EOF.
