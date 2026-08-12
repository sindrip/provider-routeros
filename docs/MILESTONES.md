# Milestones

Each milestone produces what the next consumes.
M1–M3 and M6 exit against a fresh `docker compose up`.

## M1 — Harness

Go module: boot the router, probe over REST, write evidence artifacts
stamped with the RouterOS version, tear down.

Exit: one command reproduces a trivial artifact (the version) from a clean
checkout.

## M2 — Inventory

Walk the console tree. Verdict per path: menu or not, rows or singleton,
writable, add, move. Unknowns stay unknown.

Exit: every path has a verdict.

## M3 — Menu behaviour

Per-menu probes: fields, booleans, default expansion, uniqueness, ordering,
fixed-membership keys. Main's probes are prior art; its evidence is not —
re-pin.

Exit: every writable menu has a behaviour artifact; contradictions with M2
fail the run.

## M4 — IR

Compile the artifacts into the IR.

Exit: same artifacts, byte-identical IR; contradictions fail compilation.

## M5 — Generator

Emit one Kind per menu and the descriptors the engine reads.

Exit: generated code compiles; CRDs apply; validation rejects an `extra`
key colliding with a typed field.

## M6 — Engine

The generic reconciler: stated fields converge, rows match by content,
order via `move`, unlisted-rows policy enforced.

Exit: create → converge → drift out-of-band → re-converge, for one menu of
each class.

## M7 — Release

Exit: a tagged release installs from the registry and reconciles a router.
