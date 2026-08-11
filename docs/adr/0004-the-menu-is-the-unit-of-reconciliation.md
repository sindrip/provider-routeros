# The menu is the unit of reconciliation, not the row

A row-as-resource design needs a way to say *which row is mine* that survives
recreate, and RouterOS's own `.id` does not — it is reassigned, so 0001 fell
back to comment identity and 0002 built a sequencer on it. Both were framed as
interim. This decides the shape they were interim to: **one custom resource per
menu, holding that menu's rows as a list, not one resource per row.**

The reason is now measured rather than argued. Row identity is only safe where
the device refuses two rows with the same value, and `hack/keyprobe` asked the
router that question for every menu (`config/key-uniqueness.json`, CHR 7.23.2
arm64). Of the **259 writable row-bearing menus**:

| | menus |
|---|---|
| a field the router enforces unique | **88** |
| every candidate probed, none enforced | **71** |
| nobody has asked yet | **100** |

So row-as-resource is provably sound for 88 of 259 — a third — and for the other
171 it is either provably unsound or unverified. The 88 are also not uniformly
`name`: 81 are, 5 are `interface`, 2 are `address`, which is the pattern 0003
already met at `/ipv6/nd`.

The menu as the unit needs no key at all. The CR names the menu, its spec holds
the desired rows in order, and identity is the *set*, not the member. Nothing
has to survive a recreate because nothing addresses a row across
reconciliations.

## The comment finding: not new, but now bounded

That RouterOS does not enforce comment uniqueness was already known and already
documented — the README says the provider enforces it and "fails loudly on
ambiguity instead of guessing". What keyprobe adds is the scope. `comment` was
tested on **125 menus and the router accepted a duplicate on 120**; the other 5
were rejections that named something else entirely (`/interface/eoip` refuses a
second row because its endpoints collide, not its comment). So there is now
positive evidence that the exception does not exist anywhere we have looked,
rather than an absence of evidence that one does.

This matters here because it settles how far comment identity can be taken. The
existing mitigation is *detect and fail*: a resolve that matches two rows stops
rather than guessing, which is the right behaviour and also an admission that
the situation is reachable. A hand-added rule sharing a managed rule's comment
wedges that resource, and nothing the provider does can prevent it — only
notice it. 0003 already stated the preference this implies: *a router-enforced
natural key beats a uniqueness rule only this provider knows about.* Where
there is no natural key, the remaining move is to stop needing one.

`schema/ir_test.go:TestCommentIsNeverADeviceEnforcedKey` will fail if a RouterOS
release starts enforcing comments, at which point row identity becomes cheap for
those menus and this trade-off is worth revisiting.

## Decision

- One CR per menu. `spec.forProvider` carries the rows as an ordered list; the
  external name is the menu path.
- Order is expressed by list position. That subsumes 0001's chain-as-resource
  for the **44 writable ordered menus** and retires 0002's comment sequencer for
  them: a first-match chain's order is the list's order, with no `.id`, no
  anchor, and no comment resolution.
- Within a menu, a row is addressed by its index in the desired list, resolved
  against the device by matching on the fields the spec sets. Comment identity
  survives only as an optimisation where a comment happens to be unique, never
  as the mechanism.
- Menus with a proven key (88) may *additionally* expose a row-scoped CR later.
  This ADR does not forbid it; it stops it being the default, and requires the
  key to come from `Identity.Tested`, never from a field's name looking like an
  identifier.

## Considered Options

- **Row-as-resource everywhere, with comment identity as the universal key.**
  The status quo and what 0001/0002 built toward. Now measurable: it requires a
  uniqueness the device declines to enforce on all 120 menus tested, and it
  spreads one invariant (chain order) across N independently reconciled objects,
  which 0001 already objected to on its own terms.
- **Row-as-resource only for the 88 proven menus, menu-as-resource for the
  rest.** Coherent, and where this probably ends up eventually. Rejected *as the
  starting point* because it makes the CR shape depend on a probe result that is
  itself incomplete — 100 menus are unasked, so today's split would move as
  evidence arrives, and a CR's kind is not something that can change under a
  user.
- **Menu-as-resource only for ordered menus.** The narrow version, and the one
  0001 sketched. It leaves 215 unordered writable menus on comment identity for
  no reason other than that they lack a `move` command, which has nothing to do
  with whether their rows are identifiable.
- **Wait for per-row identity to be fully probed.** The remaining 100 menus need
  a row to be creatable on the probe device, and CHR cannot create rows for
  hardware it does not have. Some of that 100 is not closeable on any single
  device, so this is waiting for something that will not arrive.

## Consequences

- **One menu means one owner.** Two compositions cannot each contribute a
  firewall rule to the same chain without an aggregation layer above the CR.
  This is the real cost, and it is a genuine regression against row-as-resource
  for multi-tenant composition. It is accepted because the alternative trades it
  for an ordering invariant that no single object owns.
- **Adoption needs an explicit tolerate-vs-prune policy**, per menu and per CR.
  A menu resource that owns the whole list has to decide what an unlisted row
  on the device means: pre-existing config to leave alone, or drift to remove.
  0002 already flagged this for chains and it now applies to all 259. Defaulting
  to prune would delete a user's hand-added rules on first apply; defaulting to
  tolerate means the CR does not actually converge. Neither default is safe, so
  it is a required field.
- **The first destructive prune needs an exact preview and approval.** Merely
  spelling `prune` in a new object is not sufficient authority to delete an
  existing menu: the controller reports a compact plan and a hash covering the
  selected connection and planned operations, then requires that hash in an
  approval annotation. The fresh plan is checked again immediately before its
  first mutation. Once a connection has been adopted, ordinary reconciliation
  remains automatic; repointing its ProviderConfig or credentials requires a
  new adoption.
- **Device-owned dynamic rows are outside static menu ownership.** A prune
  ignores rows RouterOS reports as dynamic rather than trying to match or
  delete runtime state. Their presence also prevents the planner from using a
  whole-menu block move, so they are treated like tolerated anchors while
  static rows are ordered.
- **The 44 ordered menus get correct ordering for the first time.** 0002's
  bounded window — a rule created after its chain's drop sits dead until the
  sequencer's next reconcile — closes, because there is no separate sequencer to
  run second.
- **Large menus become large objects — measured, and survivable.** With the CR
  shape this ADR implies — rows as an atomic list in spec, mirrored in status
  with a per-row condition — a v1.36.2 API server on default-limit etcd stores
  a realistic-density `/ip/firewall/filter` at ~400 bytes a row, and the first
  refusal comes at 3873 rows; rows with all 63 writable fields set hit it at
  443 (`hack/sizeprobe`, pinned in `config/menu-object-size.json`). Two
  findings shape the design. The write that dies is the *status* one, since it
  carries spec plus the observed mirror and the mirror is the larger half —
  and a floor, because a live router returns defaulted fields on top — so
  status is where the shape gives under pressure, and compacting the mirror is
  the lever. And managedFields stays flat at any row count, because an atomic
  list is one entry; atomic is forced anyway, position being identity.
- **The bridge cannot express this**, exactly as 0001 found. This is a native-
  provider decision; the upjet bridge keeps 0001 and 0002 for its lifetime, and
  those ADRs stay in force rather than being superseded.
- Row-level status becomes list-level. A single bad row fails the whole menu's
  apply, so per-row conditions have to be reported in status rather than
  inferred from separate objects' readiness.

## Evidence

Every number above is reproducible from the pinned artifacts:

```
cd schema && go run ./cmd/buildir -config ../config    # rebuild the IR
go test ./...                                          # the gates that pin these counts
```

- `config/key-uniqueness.json` — per-field uniqueness verdicts, and the note
  recording that two consecutive runs agreed on 404 of 407.
- `config/menu-object-size.json` — what a menu CR costs stored and where a
  default-limit API server refuses one, per row shape (`hack/sizeprobe`, which
  reruns it against a disposable envtest control plane).
- `schema/ir.json` — `identity.tested` per menu, `identity.key` where proven.
- `hack/keyprobe` — how the verdicts were obtained, including why a rejection
  that does not name the field is `AMBIGUOUS` rather than `UNIQUE`.
