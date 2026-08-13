# The menu is the unit of reconciliation, not the row

Row-as-resource needs a row identity that survives out-of-band delete and
recreate, and RouterOS has none: `.id` is reassigned, only 88 of 259 writable
row-bearing menus have a device-enforced unique field, and comment uniqueness
is unenforced on all 120 menus tested (main branch,
`config/key-uniqueness.json`, CHR 7.23.2). So the resource is the menu: spec
holds the desired rows as an ordered list, identity is the set, and ordering
— which RouterOS expresses only as list order plus `move` — has exactly one
owner.

Consequences: one menu, one owner (row contributions from multiple objects
need an aggregation layer above the CR); a required policy for device rows
the spec doesn't list, since no default is safe — prune deletes hand-added
config, tolerate never converges; and a measured object-size ceiling of
~3.9k realistic rows (`config/menu-object-size.json`).
