# Every menu is its own typed Kind

One generated Kind per menu (group = first path segment, kind = remaining
path CamelCased + `Menu`, namespaced only), all driving one hand-written
generic reconciler through a generated descriptor. A single free-form `Menu`
kind was rejected: a bad field in an atomic rows list wedges the whole menu's
apply, so admission validation is the compensating control — and menus are
privilege boundaries on a router (`/user` vs `/ip/dns/static`), visible to
RBAC only as distinct Kinds. Typed-first is also the reversible direction: a
generic escape kind can be added later; migrating users off one cannot.

The shape of the generated API, decided together with this:

- Probed booleans are `*bool` (omitted ≠ false); every other field is a
  string. The router's stated vocabularies become documentation, not
  validation: the IR cannot prove enum multiplicity, and a pinned enum must
  never reject what a newer router accepts.
- Rows carry `extra: map[string]string` for fields the IR has not seen;
  generated validation rejects keys colliding with typed fields.
- Read-only lists never become Kinds — their size and churn are
  device-decided; telemetry is the collector's. Read-only singletons wait
  for a consumer. Fixed-membership menus (no `add`) are generated only once
  a probe proves their key.
