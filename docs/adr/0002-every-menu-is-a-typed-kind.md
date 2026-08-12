# Every menu is its own typed Kind

Each menu becomes one generated Kubernetes Kind (group = first path segment,
kind = remaining path CamelCased + `Menu`; namespaced only), parameterizing a
single hand-written generic reconciler through a generated per-menu
descriptor. One free-form `Menu` kind was rejected: with an atomic rows list,
one bad field wedges the whole menu's apply, so admission-time validation is
the compensating control — and on a router, menus are privilege boundaries
(`/user` is not `/ip/dns/static`), which only distinct Kinds let RBAC see.
Typed-first is also the reversible direction: a generic escape kind can be
added later, while moving users off a generic kind onto typed ones is a
breaking kind migration.

## Consequences

- Fields are typed honestly: probed booleans as `*bool` (omitted ≠ false),
  everything else as strings with the router's stated vocabulary as
  documentation, not validation — the IR cannot prove enum multiplicity, and
  a pinned enum must never reject input a newer router accepts.
- Each row carries `extra: map[string]string` for fields the IR has not seen
  (newer RouterOS, hardware the probe device lacks), with generated
  validation rejecting keys that collide with typed fields.
- Read-only lists never become Kinds (device-decided size, counter churn —
  telemetry belongs to the collector). Read-only singletons may be added
  later when a consumer exists. Fixed-membership menus (no `add`) are
  generated only once a probe proves their key by set-collision.
