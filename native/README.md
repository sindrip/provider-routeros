# Native firewall-filter slice

This module is the first end-to-end, menu-native controller. It is deliberately
separate from the shipping Upjet provider while the native API is exercised:

- `FirewallFilterMenu` declares the ordered `/ip/firewall/filter` menu as one
  Kubernetes object.
- The 63 writable fields discovered by the RouterOS 7.23.2 IR are explicit in
  the CRD. Boolean pointers distinguish omitted from explicit `false`; other
  values stay strings because RouterOS accepts ranges, lists, negation, and
  unit-bearing expressions.
- `FirewallFilterMenu`, `routeros.m.sindrip.io/v1alpha1` `ProviderConfig`, its
  Secret, and the resulting `ProviderConfigUsage` are all namespaced. The menu
  resolves only the `ProviderConfig` in its own namespace, and that config must
  select Secrets in the same namespace. `ClusterProviderConfig` is not
  accepted. The endpoint, timeout, and TLS policy live in `ProviderConfig`;
  username and password are ordinary keys in its credential Secret.
- The ProviderConfig controller maintains `status.users` from namespaced
  `ProviderConfigUsage` objects and blocks deletion while any usage remains.
  ProviderConfig and Secret events immediately enqueue their referencing
  firewall menus; the periodic poll remains as a drift-recovery backstop.
- The REST planner handles matching, ambiguity, create/delete, and ordering.
  Normal status contains only each desired position's RouterOS `.id` plus a
  `Ready` condition; it does not mirror the full menu. A pending destructive
  adoption adds a bounded deletion preview.
- RouterOS `dynamic=true` rows are device-owned: they are neither adopted nor
  pruned, even when the object owns every static firewall rule.
- A first `Prune` that would delete existing static rows stops at a compact
  preview and requires approval of a plan hash before it writes anything.
- The oldest object resolving to a RouterOS endpoint is the active owner across
  all namespaces. A second object reports `OwnershipConflict` instead of
  fighting the first one, even when it uses a different namespaced
  `ProviderConfig`. Use one canonical URL for each router; aliases such as a DNS
  name and an IP address cannot be proven to identify the same device without
  contacting it.

The module targets Go 1.27 because the REST client intentionally uses strict
`encoding/json/v2`. It does not change the root provider's Go version or
binary.

## Build and install the provider package

The native controller is packaged separately as `provider-routeros-native`
while it is developed alongside the shipping Upjet provider. Build a Linux
controller image for the host architecture and embed it in a Crossplane
package with:

```sh
make package VERSION=dev
```

This writes
`../.work/native/provider-routeros-native-dev-<architecture>.xpkg`. For a
multi-architecture release, build both Linux variants and push them together
as one OCI package tag:

```sh
make package-all VERSION=v0.28.0-alpha.2
crossplane xpkg push \
  --package-files ../.work/native/provider-routeros-native-v0.28.0-alpha.2-amd64.xpkg \
  --package-files ../.work/native/provider-routeros-native-v0.28.0-alpha.2-arm64.xpkg \
  ghcr.io/sindrip/provider-routeros-native:v0.28.0-alpha.2
```

The current public prerelease can be installed on Crossplane v2 with
`kubectl apply -f config/install/provider.yaml`, or directly:

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-routeros-native
spec:
  package: ghcr.io/sindrip/provider-routeros-native:v0.28.0-alpha.2
```

The package owns only the namespaced `FirewallFilterMenu`, `ProviderConfig`,
and `ProviderConfigUsage` CRDs. It does not contain or register a
`ClusterProviderConfig`. Crossplane creates the controller Deployment and
grants the provider access to its packaged CRDs and credential Secrets; the
controller itself enforces same-namespace ProviderConfig and Secret
references.

Do not install this package alongside the shipping Upjet provider yet. Both
packages own the same namespaced `ProviderConfig` and `ProviderConfigUsage`
GVKs, so Crossplane will correctly reject the second package's attempt to take
ownership of those CRDs. Use the native package on a fresh test cluster or
after deliberately migrating away from the shipping provider.

## Try it

Use a test router. Its user needs the `api` and `rest-api` RouterOS policies.
Install the three namespaced CRDs, then run the controller against the current
kubeconfig:

```sh
kubectl apply -f config/crd/
go run ./cmd/provider
```

The referenced `ProviderConfig` must be in the same namespace as the menu and
all of its Secrets. A matching credential Secret and config are included at
`config/samples/routeros_v1alpha1_providerconfig.yaml`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: routeros-credentials
  namespace: crossplane-system
type: Opaque
stringData:
  username: admin
  password: replace-me
---
apiVersion: routeros.m.sindrip.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
  namespace: crossplane-system
spec:
  endpoint: https://192.0.2.1
  credentials:
    secretRef:
      name: routeros-credentials
  tls:
    insecureSkipVerify: true
  requestTimeout: 30s
```

The credential Secret defaults to the `username` and `password` keys. Use
`usernameKey` or `passwordKey` under `secretRef` when an existing Secret uses
different names. For private PKI, replace `insecureSkipVerify` with a local CA
reference such as `caSecretRef: {name: routeros-ca, key: ca.crt}`. Neither
credential nor CA references can name another namespace.

In another shell, edit the sample's ProviderConfig name or namespace if needed
and apply it:

```sh
kubectl apply -f config/samples/ip_v1alpha1_firewallfiltermenu.yaml
kubectl get firewallfiltermenus.ip.routeros.m.sindrip.io -n crossplane-system
kubectl get firewallfiltermenu native-smoke-test -n crossplane-system -o yaml
```

The sample uses `Tolerate` and creates a **disabled** rule, so it neither prunes
existing firewall rows nor changes packet handling. The resource defaults to
`deletionPolicy: Orphan`; deleting it leaves that test row on the router.

`unlisted: Prune` means the object owns every static firewall-filter row and
deletes every unlisted static row. RouterOS dynamic rows remain untouched.

The first destructive prune reports `Ready=False`, reason `AdoptionPending`,
and a `status.pendingPlan` containing operation counts, up to twenty rows that
would be deleted, and an approval token. Review it before approving:

```sh
kubectl get firewallfiltermenu input-firewall -n crossplane-system \
  -o jsonpath='{.status.pendingPlan}'

APPROVAL_TOKEN="$(kubectl get firewallfiltermenu input-firewall -n crossplane-system \
  -o jsonpath='{.status.pendingPlan.approvalToken}')"

kubectl annotate firewallfiltermenu input-firewall -n crossplane-system \
  firewallfiltermenus.ip.routeros.m.sindrip.io/approve-prune="$APPROVAL_TOKEN"
```

The token covers the selected connection and exact planned operations. If the
router configuration or credentials change before apply, the controller
rejects it and publishes a new preview. Live `bytes` and `packets` counters do
not change the token. Once adopted, later spec edits reconcile automatically.
Repointing the ProviderConfig or Secret requires adoption again.

`deletionPolicy: Delete` is admitted only together with `Prune`, and deleting
that object removes every static row; dynamic rows remain device-owned. Both
destructive choices are intentionally explicit. `Orphan` remains the
recommended deletion policy so an accidental Kubernetes deletion cannot empty
the firewall.

## Develop

```sh
go test ./...
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.20.1 \
  object paths=./api/... \
  crd paths=./api/... output:crd:artifacts:config=config/crd
```

The envtest package skips unless `KUBEBUILDER_ASSETS` is set. For example:

```sh
KUBEBUILDER_ASSETS="$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.24.1 use -p path 1.36.2)" \
  go test ./internal/integration -v
```
