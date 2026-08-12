# Lab

Disposable RouterOS ([`mikrotik/chr`](https://hub.docker.com/r/mikrotik/chr))
for probes, integration, and e2e tests. Needs Docker.

```sh
docker compose up --wait    # blocks until REST answers
curl -su admin: http://127.0.0.1:18080/rest/system/resource
docker compose down         # discards router state
```

From inside the lab network the router is `172.31.99.11`.
