# Sketches

Pinned ideas, not decisions. Delete when built or refuted.

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
