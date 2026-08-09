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
Four classes exist:

- **Name identity** (66 resources, see `config/name_identity.go`): resources
  whose RouterOS name is enforced unique — verified per resource by
  `hack/uniqprobe`, verdicts pinned in `config/name-uniqueness.json` — are
  identified by name. The external-name is the RouterOS name, and the current
  internal id is resolved on every operation, so out-of-band delete/recreate
  heals on the next reconcile. For DHCP clients the router supports a settable,
  enforced-unique `name` that the upstream Terraform schema does not model; the
  provider injects the field (`spec.forProvider.name`, required). Certificates
  are here too, despite hand-written upstream CRUD: it passes the id straight
  through as a RouterOS `number`/`numbers` argument, which resolves a name just
  as well as a `*XX`.
- **Comment identity** (firewall filter, mangle, raw and NAT rules — IPv4 and
  IPv6 — bridge filter rules, bridge ports, bridge VLAN entries, interface
  list members, DHCP server leases, DNS records, BGP connections,
  instances, templates and VPNs, and OSPF areas, see
  `config/comment_identity.go`):
  these items have no enforced-unique name — DNS records have a name, but
  RouterOS allows duplicates for round-robin, and the BGP menus accept
  duplicate names outright — so the comment is the identity. It is required at create, must be unique within the menu
  (RouterOS does not enforce this; the provider does, and fails loudly on
  ambiguity instead of guessing), and renaming it moves the external-name
  along.
- **Interface identity** (IPv6 neighbour discovery, see
  `config/interface_identity.go`): menus the router keeps at one item per
  interface are identified by that interface, which RouterOS enforces unique
  itself. `/ipv6/nd` also ships one row it owns — `default=true`, covering
  `interface=all` — that can be neither added nor removed, so create adopts it
  in place and delete releases it and leaves it standing. See `docs/adr/0003`.
- **Factory-name identity** (ethernet interfaces, see
  `config/factory_identity.go`): physical ports are identified by the
  immutable `default-name` (`spec.forProvider.factoryName`, e.g. `ether8`),
  which survives renames, configuration resets, and reinstalls — the `name`
  field is mutable and managed by the resource itself, and the internal id is
  reassigned on rebuild, where it could resolve to the wrong physical port.
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

One class of upstream schema bug is corrected at runtime rather than pinned:
fields whose upstream schema carries an SDK default for an argument the router
does not have (`config/phantom_defaults.go`). A defaulted field is serialized
into every create, and RouterOS rejects requests carrying unknown parameters,
so such a resource cannot be created at all — BGP connections and templates
were unusable because of `add_path_out`. The runtime drops the default so the
field is only sent when explicitly set; membership is judged at the
serializer, since upstream's version-drift table legitimately renames some
router-absent fields (`address_families` → `afi`).

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

When a resource kind switches identity class — or when a release changes what
belongs in `spec.forProvider` — existing managed resources must be migrated
before upgrading:

- **v0.2.0** — VLANs, users and user groups: the first kinds to leave `*XX`
  identity. Rewrite the external-name annotation from the `*XX` id to the
  item's RouterOS name.
- **v0.3.0** — name identity extends to every probe-verified resource, sixty
  in one release (the list grows from 3 to 63, gated on the pinned UNIQUE
  verdicts in `config/name-uniqueness.json`). Rewrite the external-name
  annotation from the `*XX` id to the item's name for any managed resource of
  a newly flipped kind.
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
- **v0.8.0** — interface lists: the kinds `List` and `ListMember` (interface
  group) are renamed to `InterfaceList` and `InterfaceListMember`. The bare
  kind `List` collides with the Kubernetes core list wrapper and cannot be
  routed by kubectl or GitOps machinery at all, so no working CRs of these
  kinds can exist; recreate any manifests under the new kinds. In the same
  sweep `Service` (ip), `Secret` (ppp), and `Configuration` (capsman, wifi)
  become `IPService`, `PPPSecret`, `CAPsMANConfiguration`, and
  `WifiConfiguration` — functional before, but shadowed by core or Crossplane
  kinds for kubectl and kind-keyed tooling.
- **v0.9.0** — ethernet interfaces: rewrite the external-name annotation from
  the `*XX` id to the port's factory name (`spec.forProvider.factoryName`,
  e.g. `ether1`).
- **v0.10.0** — DNS records: give each managed record a unique comment and
  rewrite the external-name annotation from the `*XX` id to the comment. The
  record name cannot be the identity because RouterOS allows same-name
  records (round-robin).
- **v0.11.0** — interface list members: give each managed member a unique
  comment and rewrite the external-name annotation from the `*XX` id to the
  comment. Dynamic members carry no comment and are never matched or adopted.
- **v0.12.0** — DHCP server leases: give each managed static lease a unique
  comment and rewrite the external-name annotation from the `*XX` id to the
  comment. Leases have no name field at all — only the (server, address,
  mac-address) tuple and the ephemeral `*XX` id.
- **v0.14.0** — BGP connections, instances and templates: give each managed
  row a unique comment and rewrite the external-name annotation from the
  `*XX` id to the comment. The name cannot be the identity because RouterOS
  accepts duplicate names throughout the BGP menus (verified by probe) and
  even resolves references to a duplicated instance name ambiguously.
- **v0.15.0** — firewall filter, mangle and raw rules (IPv4 and IPv6), IPv6
  NAT rules, and bridge filter rules: give each managed rule a unique comment
  and rewrite the external-name annotation from the `*XX` id to the comment.
  Same keyless-ordered-menu shape as the NAT rules fixed in v0.5.0.
  In the same release, name identity extends to wireguard peers, VXLAN
  interfaces, PPPoE server bindings, DHCP server options, option sets and
  matchers, and DNS forwarders (all probe-verified UNIQUE): rewrite the
  external-name annotation from the `*XX` id to the item's name.
  BGP VPNs and OSPF areas accept duplicate names (probe-verified) and join
  comment identity instead: give each managed row a unique comment and
  rewrite the external-name annotation from the `*XX` id to the comment.
  The BGP VPN comment field is injected — RouterOS has it, the upstream
  schema does not.
- **v0.16.0** — routing filter rules: give each managed rule a unique comment
  and rewrite the external-name annotation from the `*XX` id to the comment.
- **v0.17.0** — comment-addressed sequencing arrives (`docs/adr/0002`). No
  migration is forced: a `spec.sequence` entry beginning with `*` still passes
  through as a literal id. But raw ids are unknowable in Git and reassigned on
  recreate, so move existing sequences onto the rules' comments — which is the
  whole point of the sequencer. Comments must not begin with `*`.
- **v0.18.0** — IP and IPv6 addresses, DHCP server networks and DHCPv6
  clients: give each managed row a unique comment and rewrite the
  external-name annotation from the `*XX` id to the comment. These menus have
  no name at all, so the ephemeral id was their only identity. The comment
  field is native to all four — no CRD change.
- **v0.20.0** — system clock: `date` and `time` are readings that advance on
  their own and are no longer late-initialized. The fix stops new resources
  acquiring them, but a clock resource reconciled by an earlier release
  already has a stale instant frozen in `spec.forProvider`, and will keep
  writing the clock backwards until it is removed. Delete `date` and `time`
  from `spec.forProvider` on existing clock resources.
- **v0.21.0** — certificates: rewrite the external-name annotation from the
  `*XX` id to the certificate's name (probe-verified UNIQUE). In the same
  release, deleting a CA-issued certificate removes it. Upstream only revoked
  it, leaving the row in place holding its unique name, which blocked every
  recreate; revocation still happens first, then the row is removed. Any
  certificate stranded that way by an earlier release is still on the router
  with `revoked=true` and must be deleted by hand before the name can be
  reused.
  IPv6 neighbour discovery joins interface identity: the `interface=all` row
  the router ships is adopted rather than added, so it no longer has to be
  left unmanaged. Set the external-name to the interface.
- **v0.22.0** — IPv6 neighbour discovery: `spec.forProvider.advertiseDns`
  changes type from boolean to string, the first generated field whose type
  this provider has had to correct. Rewrite `true` as `"yes"` and `false` as
  `"no"`; a manifest left holding a boolean will not validate. RouterOS 7.23
  takes `self|yes|no` there and rejects `true|false` outright, so the boolean
  could neither set `self` — the setting the argument mostly exists for — nor
  observe it: a device holding `self` read back as `false`, never matched its
  spec, and was overwritten with `yes` on every reconcile. The field also
  loses its schema default, so a spec that omits `advertiseDns` no longer
  writes one; if you were relying on the implicit `yes`, set it explicitly.
- **v0.23.0** — the same correction, swept rather than stumbled on:
  `hack/typedump` asks a live router what every console property accepts and
  `hack/schemaaudit/mistyped.py` diffs that against the upstream schema, which
  turns "this boolean is not a boolean" from a hunch into a query. Six more
  fields change type from boolean to string, and a manifest left holding a
  boolean will not validate. Rewrite `true` as `"yes"` and `false` as `"no"`
  in `spec.forProvider` for `usePeerDns` on OVPN clients (`exclusively|yes|no`),
  `pfs` on SSTP clients and servers (`required|yes|no`), and `useRadius` on
  IPv6 DHCP servers (`accounting|yes|no`). `digestAlgorithm` on certificates
  needs no manifest change — it is observed and never written, and only stops
  reporting `false` against a router holding `sha256`. `loopProtectStatus` on
  MACVLANs needs the opposite of a rewrite: RouterOS answers `unknown
  parameter loop-protect-status` to any write of it, so delete it from
  `spec.forProvider` rather than translating it and read it from
  `status.atProvider` instead.

The releases absent from that list need nothing: v0.1.0 predates the identity
work, v0.13.0 fixed a create path that could never have succeeded, so no
managed resource of those kinds existed to migrate, and v0.19.0 changed only
what a read observes.

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
