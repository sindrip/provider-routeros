# Native firewall-filter slice

This module is the first end-to-end, menu-native controller. It is deliberately
separate from the shipping Upjet provider while the native API is exercised:

- `FirewallFilterMenu` declares the ordered `/ip/firewall/filter` menu as one
  Kubernetes object.
- The 63 writable fields discovered by the RouterOS 7.23.2 IR are explicit in
  the CRD. Boolean pointers distinguish omitted from explicit `false`; other
  values stay strings because RouterOS accepts ranges, lists, negation, and
  unit-bearing expressions.
- The existing cluster-scoped `routeros.sindrip.io/v1beta1` `ProviderConfig`
  and Secret JSON are reused. This slice currently accepts the `Secret`
  credential source and an `http` or `https` `hosturl`. It creates the normal
  `ProviderConfigUsage`, so Crossplane will not remove a configuration that is
  still referenced.
- The REST planner handles matching, ambiguity, create/delete, and ordering.
  Status contains only each desired position's RouterOS `.id` plus a `Ready`
  condition; it does not mirror the full menu.
- The oldest object referencing a ProviderConfig is the active owner. A second
  object reports `OwnershipConflict` instead of fighting the first one.

The module targets Go 1.27 because the REST client intentionally uses strict
`encoding/json/v2`. It does not change the root provider's Go version or
binary.

## Try it

Use a test router. Its user needs the `api` and `rest-api` RouterOS policies.
Install the CRD, then run the controller against the current kubeconfig:

```sh
kubectl apply -f config/crd/ip.routeros.sindrip.io_firewallfiltermenus.yaml
go run ./cmd/provider
```

The referenced `ProviderConfig` must be the existing cluster-scoped kind:

```yaml
apiVersion: routeros.sindrip.io/v1beta1
kind: ProviderConfig
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

In another shell, edit the sample's ProviderConfig name if needed and apply it:

```sh
kubectl apply -f config/samples/ip_v1alpha1_firewallfiltermenu.yaml
kubectl get firewallfiltermenus.ip.routeros.sindrip.io
kubectl get firewallfiltermenu native-smoke-test -o yaml
```

The sample uses `Tolerate` and creates a **disabled** rule, so it neither prunes
existing firewall rows nor changes packet handling. The resource defaults to
`deletionPolicy: Orphan`; deleting it leaves that test row on the router.

`unlisted: Prune` means the object owns the complete firewall-filter menu and
deletes every unlisted row. `deletionPolicy: Delete` is admitted only together
with `Prune`, and deleting that object empties the complete menu. Both are
intentionally explicit and should be used only on a disposable router while
this slice is experimental.

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
