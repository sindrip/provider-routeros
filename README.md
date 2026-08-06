# Provider RouterOS

`provider-routeros` is an [Upjet](https://github.com/crossplane/upjet)-generated
[Crossplane](https://crossplane.io/) provider for MikroTik RouterOS, built as
an adapter around the official
[`terraform-routeros`](https://github.com/terraform-routeros/terraform-provider-routeros)
provider. It generates all 254 upstream Terraform resources in both
cluster-scoped and namespaced Crossplane API families.

Where it deliberately diverges from upstream is resource identity: Terraform's
episodic apply model tolerates RouterOS's ephemeral internal `*XX` ids, but a
Crossplane controller reconciling forever does not — RouterOS reassigns the id
when an item is deleted and recreated outside the controller, which permanently
breaks reconciliation and can silently mint duplicates. Every divergence is
explicit, probe-verified against a live RouterOS instance, and pinned in this
repository.

## Resource identity

The `crossplane.io/external-name` annotation holds the resource identity.
Three classes exist:

- **Name identity** (65 resources, see `config/name_identity.go`): resources
  whose RouterOS name is enforced unique — verified per resource by
  `hack/uniqprobe`, verdicts pinned in `config/name-uniqueness.json` — are
  identified by name. The external-name is the RouterOS name, and the current
  internal id is resolved on every operation, so out-of-band delete/recreate
  heals on the next reconcile. For DHCP clients the router supports a settable,
  enforced-unique `name` that the upstream Terraform schema does not model; the
  provider injects the field (`spec.forProvider.name`, required).
- **Comment identity** (firewall NAT, bridge ports, and bridge VLAN entries,
  see `config/comment_identity.go`): these items have no name, so the comment
  is the identity. It is required at create, must be unique within the menu
  (RouterOS does not enforce this; the provider does, and fails loudly on
  ambiguity instead of guessing), and renaming it moves the external-name
  along.
- **Provider identity** (everything else): the upstream Terraform id, usually
  the ephemeral `*XX`. Keep external names in Git for resources that must be
  re-adopted after rebuilding the management cluster, and prefer `Observe`
  management policies when adopting existing configuration.

Rule ordering in ordered menus (firewall chains, queues) is out of scope for
this provider; see `docs/adr/0001` for the design that addresses it and the
reasoning.

## Verified against the router, not the docs

RouterOS behavior is pinned from probing disposable CHR instances, not from
documentation, which the probes have shown to be wrong in both directions:

- `config/console-tree.json` — the router's own menu/command/argument tree
  from `/console/inspect` (`hack/inspectdump`), the closest thing RouterOS has
  to a REST API schema.
- `config/name-uniqueness.json` — per-resource name-uniqueness verdicts
  (`hack/uniqprobe`).
- `config/type-verdicts.json` — accepted values and coercions of disputed
  field types (`hack/schemaaudit/typeprobe.py`).
- `hack/chr/run.sh` — boots the disposable CHR under qemu that all probes run
  against; `hack/schemaaudit/audit.py` diffs router truth against the upstream
  provider schemas.

## Runtime model

The upstream Terraform Plugin SDK v2 provider is compiled into the Crossplane
provider process (Upjet "no-fork" mode). The runtime image contains neither the
Terraform CLI nor a separate provider plugin. Resource operations are delegated
to the official provider callbacks and schemas, wrapped by the identity layer
described above.

## Configuration

Create a JSON credential secret and reference it from either a `ProviderConfig`
or `ClusterProviderConfig`. The JSON is passed to the official provider and may
contain any of its configuration fields:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: routeros-creds
  namespace: crossplane-system
type: Opaque
stringData:
  credentials: |
    {
      "hosturl": "https://192.0.2.1",
      "username": "admin",
      "password": "replace-me",
      "insecure": false
    }
---
apiVersion: routeros.m.sindrip.io/v1beta1
kind: ClusterProviderConfig
metadata:
  name: default
spec:
  credentials:
    source: Secret
    secretRef:
      name: routeros-creds
      namespace: crossplane-system
      key: credentials
```

Supported keys are `hosturl`, `username`, `password`, `ca_certificate`,
`insecure`, `suppress_syso_del_warn`, `routeros_version`, and `rest_timeout`.
`ca_certificate` has the same meaning as upstream: it is a path visible inside
the provider container.

## Upgrading across identity changes

When a resource kind switches identity class, existing managed resources must
be migrated before upgrading:

- **v0.4.0** — DHCP clients: set `spec.forProvider.name` to the router's
  current client name and rewrite the external-name annotation from the `*XX`
  id to that name.
- **v0.5.0** — firewall NAT rules: give each managed rule a unique comment and
  rewrite the external-name annotation from the `*XX` id to the comment.
- **v0.6.0** — bridge ports: give each managed port a unique comment and
  rewrite the external-name annotation from the `*XX` id to the comment.
- **v0.7.0** — bridge VLAN entries: give each managed entry a unique comment
  and rewrite the external-name annotation from the `*XX` id to the comment.
  Dynamic VLAN rows carry no comment and are never matched or adopted.

## Following upstream

The exact official provider release is pinned as a Go module in `go.mod`.
Renovate proposes every stable upstream release. The Makefile derives the
Terraform schema and documentation tag from that module version, so an update
is reviewed by regenerating metadata, APIs, controllers, and CRDs and checking
the resulting diff. Tests gate every identity override against the pinned
verdicts and flag overrides that upstream has made redundant.

## Development

Generate the provider from the pinned upstream release:

```console
make generate
```

Run tests and build the provider:

```console
go test ./...
go build ./cmd/provider
```

Run the identity integration tests against a live disposable router:

```console
hack/chr/run.sh
CHR_REST=http://127.0.0.1:18080 go test -run LiveCHR ./config/
hack/chr/run.sh stop
```

Run against a Kubernetes cluster:

```console
make run
```

Please report bugs and feature requests in this repository.
