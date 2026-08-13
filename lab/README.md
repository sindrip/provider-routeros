# Lab

Disposable RouterOS ([`mikrotik/chr`](https://hub.docker.com/r/mikrotik/chr))
for probes, integration, and e2e tests. Needs Docker.

```sh
docker compose up --wait    # blocks until every router answers REST
curl -su admin: http://127.0.0.1:8011/rest/system/resource
docker compose down         # discards router state
```

Router ordinal `n` is `127.0.0.1:801n` from the host, `172.31.99.1n` from
inside the lab network. Everything derives from `n`: the container MAC is
`02:52:6f:53:00:n0`, the VM's ether1 is that +1, and dnsmasq pins its
lease.

Adding a router is a data diff: one `rN` service line, one `--dhcp-host`
pin, and its ordinal in the healthcheck loop. The relay for its port
already idles.

The chr image bridges the container NIC out of its own netns, so routers
cannot publish ports or self-healthcheck; the `net` container relays and
vouches for readiness instead. In its command, `$$n` is compose escaping
for a literal shell `$n`.
