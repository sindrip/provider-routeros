# Bridge-era rule ordering: one comment-addressed sequencer per menu

Comment identity (see the interim measure noted in 0001) answers which row is
mine but not where the row sits, and under first-match-wins a rule reconciled
after its chain's final drop is dead config that still reads Synced/Ready. The
upjet bridge cannot express 0001's chain-as-resource design, but it already
generates move/Items — upstream's sequencer, whose spec.sequence takes raw
RouterOS .id values, unknowable in git and reassigned on recreate. The
decision: the runtime rewires move/Items to accept comments in the sequence,
resolving each to the row's current .id on every operation (config/
move_sequence.go). Upstream semantics are kept — the last entry is the anchor,
preceding entries are placed before it in listed order by one /move call, read
reports managed rows in device order so reorders surface as drift. Rule
resources stay position-free; one Items resource per menu is the single owner
of that menu's order; desired order lives only in the spec.

## Considered Options

- **Position markers in rule comments** (`[tf:pos=N]`): collides with comment
  identity — the comment is the resource identity and external name, so
  reordering would rewrite identities — plus 0001's objections (renumbering,
  all-rows-must-participate, N reconcilers racing over one invariant).
- **Per-rule placeBefore**: upstream's pass-through wants a router-internal
  .id at create time and is not reconciled afterwards; also relational order
  across resources, rejected in 0001.
- **Wait for the native substrate**: leaves input-chain adoption unsafe for
  the lifetime of the bridge; the sequencer is a thin wrapper on machinery
  that already exists.

## Consequences

- Sequence entries starting with `*` pass through as literal .id values, so
  raw-id sequences keep working; comments must not start with `*`.
- Writes are strict (every comment must resolve to exactly one row) so a
  partial sequence is never applied; reads are lenient (missing rows are
  dropped, surfacing as drift) so an out-of-band delete degrades into
  recreate-then-reorder instead of a wedged read.
- Drift detection sees only the relative order of listed rows — an unmanaged
  row sliding between two managed ones changes first-match semantics without
  triggering a move. Unlisted rows are tolerated, never pruned. Both are
  inherited from upstream and acceptable for the bridge; the full answer
  remains 0001's chain resource.
- A rule created after its chain's drop sits at the bottom until the
  sequencer's next reconcile: a bounded eventual-consistency window, not a
  correctness guarantee mid-apply.
