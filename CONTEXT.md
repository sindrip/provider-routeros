# RouterOS Provider

A Kubernetes provider that reconciles MikroTik RouterOS devices from typed,
per-menu custom resources, generated from observations probed out of live
routers.

## Language

**Menu**:
A RouterOS console path (`/ip/firewall/filter`) that bears rows or one
settings record — the router's own partition of its configuration space, and
the unit of reconciliation. Non-leaf containers (`/ip/firewall`) and
action-only nodes (`/tool/ping`) are not menus.
_Avoid_: table, section, config block

**Menu class**:
The behavioural shape of a menu: an ordered list (position carries meaning),
a list, or a singleton. A class is assigned from observation, never assumed.

**Row**:
One record within a list-class menu. A row is addressed by its position in
the desired list and matched against the device by its content; the device's
`.id` is a handle, not an identity.
_Avoid_: item, entry, rule (rule is one menu's vocabulary, not the model's)

**Singleton**:
A menu holding exactly one settings record. A singleton spec converges its
stated fields and tolerates the rest.

**Stated field**:
A field a spec explicitly sets. Only stated fields are converged; an unstated
field is unmanaged, not "default".

**Observation**:
A fact about RouterOS behaviour pinned by probing a live, disposable router.
Documentation is not an observation; console output formatted for humans is
presentation, not observation.

**Probe**:
A program that asks a disposable router one class of question and pins the
answers as observations.

**IR**:
The intermediate representation compiled from observations — the sole
authority the generator reads. Never hand-edited.
