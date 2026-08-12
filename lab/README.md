# Lab

Disposable RouterOS for probes, integration, and e2e tests. One compose
file; Docker is the only prerequisite.

```sh
docker compose up --wait    # pristine router, blocks until REST answers
curl -su admin: http://127.0.0.1:18080/rest/system/resource | jq .version
docker compose down         # reset is down + up
```

Versions and channels are image tags: `CHR_VERSION=7.24rc4 docker compose up`.

## Design

Nodes are MikroTik's official CHR container image
([`mikrotik/chr`](https://hub.docker.com/r/mikrotik/chr)): the CHR disk in
qemu, amd64 + arm64, KVM when `/dev/kvm` exists (CI) and TCG otherwise (this
laptop, REST in under a minute). The image bridges each container interface
to a VM NIC in order, so data-plane links between routers are just shared
compose networks, starting at `ether2`.

Management is bridged, not NATed — the router asks for its own address:

- **dhcp** (dnsmasq) answers it. The VM's MAC is the container's + 1, so a
  pinned container MAC plus `dhcp-host` fixes r1 at `172.31.99.11`.
- **gw** (socat) publishes `127.0.0.1:18080 → r1:80`, since a macOS Docker
  host only reaches addresses Docker itself assigned. Linux hosts reach the
  lease directly. gw's healthcheck is what `up --wait` blocks on.

A hand-built qemu node under containerlab preceded this and was deleted when
the official image was found; see docs/adr/0004.
