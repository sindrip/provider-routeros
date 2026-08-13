# Sketches

Pinned ideas, not decisions. Delete when built or refuted.

## Lab coverage dimensions (parked 2026-08-13)

License and package set shape the tree: free-tier CHR hides cloud menus;
all-packages exposes ~1k more paths (restraml). If baking an
all-packages lab image: p1 trial needs a mikrotik.com account + router
egress (`/system/license renew level=p1`), and 17 extra packages on a
small-RAM CHR mimics a wedge (inspect 70ms → 10s+, then dead) — give the
VM 1024MB. Both change evidence scope: grill before adopting.

## From forum #149360 (parked 2026-08-13)

restraml (tikoci.github.io/restraml) generates per-version OpenAPI from
the same inspect oracle — an independent implementation to cross-check
fields/types against. `request=syntax` via `path=` also yields per-field
descriptions ("Local IP address") — generated CRD docs for M5; note the
crash vector on reference args, use fully-qualified inputs only.

## REST addressing (parked 2026-08-13)

The device does not URL-decode paths: GET `.../%2A1` answers 400 where
`.../*1` answers 200 (probed, CHR 7.23.3). Prior art
(terraform-routeros) addresses rows by `.id` only, names via the query
string, and escapes nothing but spaces. So path escaping is impossible
and a name with `/ # ? %` is unaddressable in a path — the client should
refuse such segments loudly rather than escape them. Design deferred.
