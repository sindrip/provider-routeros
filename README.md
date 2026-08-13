# routeros

MikroTik RouterOS as a substrate: a REST client, probes that pin
observations of disposable routers, an IR compiled from them, and
generators emitting the products — crossplane provider, OTel collector.

Terms: CONTEXT.md. Decisions: docs/adr/. Plan: docs/MILESTONES.md.

## Commands

```sh
go tool task --list   # the verb menu
go test ./...         # unit tests, no docker
docker compose --project-directory lab up --wait   # a disposable router
```
