# Ordered RouterOS menus become chain-level resources, not per-rule resources

RouterOS order-sensitive menus (firewall filter/nat/mangle/raw, queue lists,
bridge filters — mechanically identifiable: their console menu exposes a
`move` command, see config/console-tree.json) carry semantics in rule order,
but the REST API has no stable position attribute; order exists only as list
order plus `move`/`place-before`. In the future generated substrate, each
ordered menu is modeled as one Crossplane resource per chain whose spec holds
the ordered rule list; the controller diffs the desired sequence against the
device and converges with minimal create/move/delete operations. Individual
rules are not separately-managed resources, and no position value is ever
stored on the device.

## Considered Options

- **Position encoded in the rule comment** (`[tf:pos=N]`, as
  ebogdum/terraform-provider-routeros does): required for Terraform because
  its CRUD is per-resource with no sibling view, so the device must carry the
  ordering hint. Crossplane has no such constraint — desired order lives
  durably in the spec — and the encoding costs comment pollution, gap
  renumbering, and all-rules-must-participate.
- **Per-rule resources with a priority field**: N independent reconcilers
  enforcing one shared invariant race each other and thrash the chain with
  `move` calls; ordering needs exactly one owning actor.
- **Linked-list references** (`insertBefore: otherRule`): makes order
  relational across resources; deleting a referenced rule breaks the chain
  and reconcile order becomes load-bearing.

## Consequences

- Granularity is the chain, not the rule: a chain is only meaningful as a
  whole under first-match-wins, but rules cannot be contributed to one chain
  from multiple compositions without an aggregation layer.
- Rule identity inside a chain is an internal concern of the diff (content
  hash or hidden tag), not a user-facing contract.
- Needs an explicit policy for rules the controller does not own
  (unmanaged/dynamic rules: tolerate vs prune) and safe diff sequencing so
  guard rules (e.g. accept-established) stay effective mid-apply.
- The current upjet-based provider cannot express this (Terraform's model is
  per-resource); ordering stays out of scope there, and NAT identity uses
  comment-keyed lookup as an interim measure.
