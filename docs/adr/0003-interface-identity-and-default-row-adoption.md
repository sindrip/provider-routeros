# Router-owned menu rows: interface identity, and create that adopts

Some menus are not a free population of rows. `/ipv6/nd` holds at most one row
per interface — the router enforces it, answering a second add with
"configuration for this interface already exists" — and ships one row it owns
outright: `default=true`, covering `interface=all`, refusing deletes with "can
not remove default rule". Upstream create is a plain add, so managing that row
was impossible; the add could only ever collide with the row already occupying
the interface, and the menu had to stay out of Crossplane entirely, where a
reset destroys it and nothing puts it back. The decision: the interface is the
identity (config/interface_identity.go), resolved to the current `.id` on every
operation, and create adopts the router-owned row in place rather than adding
it. Delete releases that row without touching the router; a non-default row
already on the interface is a collision, not something to take over. The rest
of the menu keeps ordinary create and delete semantics.

The interface is a genuine key, not a provider convention: RouterOS enforces
uniqueness itself, which is why this is neither name identity (there is no
name) nor comment identity (the rows do carry a comment, but a router-enforced
natural key beats a uniqueness rule only this provider knows about). Adoption
keys on the `default` flag and not on `interface == "all"` — the router accepts
moving the default row onto another interface, verified against 7.23.2, so the
flag is the property of the row and the interface is not.

## Considered Options

- **Hardcode the `.id`**: `*N` proved identical across reboot,
  `reset-configuration`, and a second independent fresh install, so pinning
  `*1` would work today. But the numbering is a build artifact — this build has
  no `*3` or `*5` — so it pins the provider to one image, and it says nothing
  about the addable half of the menu, where `*N` is a non-recycling counter:
  delete the `ether2` row and re-add it and the id goes `*2` → `*5`. That is
  the same reassignment that drove comment identity in 0001.
- **Leave the default row to `[Observe, Update]` management policies**: needs
  the row in state to begin with, which needs a create that cannot succeed. It
  also pushes a per-MR policy decision onto every consumer for what is a
  property of the menu, not of anyone's intent.
- **Leave the menu unmanaged**: the status quo, and the thing being fixed — a
  GAP line whose contents a reset erases with no path back.

## Consequences

- The external name is the interface. Changing `spec.forProvider.interface`
  renames the row on the router, and is rejected when the target interface is
  already occupied — the same shape as a comment rename.
- Deleting the MR for the default row releases it, leaving the row on the
  router with its last applied settings rather than reverting them. The delete
  returns a warning saying so; there is no other option, since the router will
  not remove it.
- Create can take over a row it did not make. That is deliberate and bounded to
  `default=true`, so it can never silently absorb a row somebody else placed.
- If the default row is moved onto another interface out of band, an MR for
  that interface will adopt it, and an MR for `all` will find its row gone and
  add a fresh one. The router's own flag wins, which is the honest reading.
- Runtime-only, like the other identity overrides: the generated CRDs are
  unchanged, so this needs no regeneration.
- `/ip/service` shared the same GAP line and needed none of this. It is also a
  fixed population — add and delete are both refused — but upstream already
  models create and update as `/ip/service/set` and delete as a state-only
  no-op under name identity, which is exactly the behaviour wanted. Its
  name-keyed read is ambiguous by construction (the row for the live REST
  connection duplicates the `www` name) but not in practice: static services
  are allocated at boot, dynamic rows after, so the static row sorts first.
