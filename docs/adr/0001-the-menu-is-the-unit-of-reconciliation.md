# The menu is the unit of reconciliation, not the row

A row-as-resource design needs each row to carry an identity that survives
out-of-band delete and recreate, and RouterOS does not offer one: `.id` is
reassigned, and probing every writable row-bearing menu on a live CHR found a
device-enforced unique field on only 88 of 259 — comment uniqueness, the
universal fallback, was accepted as duplicate on all 120 menus where it was
tested (main branch, `config/key-uniqueness.json`, CHR 7.23.2). One custom
resource per menu needs no row key at all: the spec holds the desired rows as
an ordered list, identity is the set, and ordering — which RouterOS expresses
only as list order plus `move` — is owned by exactly one object instead of
being an invariant smeared across N independently reconciled ones.

## Consequences

- One menu, one owner. Contributing rows to a shared menu from multiple
  objects requires an aggregation layer above the CR, not two CRs.
- Owning the whole list forces an explicit policy for device rows the spec
  does not list; no default is safe (prune deletes hand-added configuration,
  tolerate never converges). The policy's exact shape is re-derived at the
  engine layer.
- Large menus become large objects; the API server bounds this (~3.9k
  realistic rows measured on main, `config/menu-object-size.json`), and
  status is where the shape gives under pressure.
