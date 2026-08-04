# Provider RouterOS

`provider-routeros` is an [Upjet](https://github.com/crossplane/upjet)-generated
[Crossplane](https://crossplane.io/) provider for MikroTik RouterOS. It is a
direct adapter around the official
[`terraform-routeros`](https://github.com/terraform-routeros/terraform-provider-routeros)
provider: the Terraform provider's managed-resource schemas and behavior are
the source of truth.

The provider intentionally does not patch RouterOS behavior, maintain a fork,
or add a second resource model. It currently generates all 254 upstream
Terraform resources in both cluster-scoped and namespaced Crossplane API
families. Terraform's 16 read-only data sources are not Crossplane managed
resources and are not generated.

## Runtime model

The upstream Terraform Plugin SDK v2 provider is compiled into the Crossplane
provider process (Upjet "no-fork" mode). The runtime image contains neither the
Terraform CLI nor a separate provider plugin. Resource operations are delegated
to the official provider callbacks and schemas.

Terraform SDK `ValidateFunc` callbacks are Terraform configuration-time
validation and are not run while Upjet refreshes state in no-fork mode. This is
why values accepted by RouterOS, such as `romon`, do not require a local schema
patch.

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

## Resource identity and recovery

Upjet stores the official Terraform resource ID in the
`crossplane.io/external-name` annotation. The provider does not invent natural
keys, inject ownership comments, or automatically rediscover every RouterOS
object after Kubernetes state is lost. Keep external names in Git for resources
that must be re-adopted after rebuilding the management cluster, and first use
Crossplane management policies such as `Observe` when adopting existing
configuration.

Ordering, singleton behavior, defaults, move operations, and deletion semantics
remain those implemented by `terraform-routeros`. Any future custom behavior
should be explicit and should not silently diverge from upstream.

## Following upstream

The exact official provider release is pinned as a Go module in `go.mod`.
Renovate proposes every stable upstream release. The Makefile derives the
Terraform schema and documentation tag from that module version, so an update is
reviewed by regenerating metadata, APIs, controllers, and CRDs and checking the
resulting diff.

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

Run against a Kubernetes cluster:

```console
make run
```

Please report bugs and feature requests in this repository. Behavior inherited
from `terraform-routeros` should be followed upstream rather than patched here.
