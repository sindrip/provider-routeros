package rest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Unlisted is what to do with a device row that no desired row claims.
//
// There is deliberately no zero value that means anything: prune deletes
// configuration a person added by hand the first time a menu resource is
// applied, and tolerate means the resource never actually converges to its
// spec. Neither is safe as a default, so ADR 0004 makes the choice required and
// Plan refuses a spec that has not made it.
type Unlisted string

const (
	// UnlistedTolerate leaves rows alone. The menu converges to "contains the
	// desired rows, in the desired relative order" rather than "is the desired
	// rows".
	UnlistedTolerate Unlisted = "tolerate"
	// UnlistedPrune deletes them. The menu converges to exactly the spec.
	UnlistedPrune Unlisted = "prune"
)

// MenuSpec is the menu being reconciled and the policy for doing so.
type MenuSpec struct {
	// Path is the menu, e.g. "/ip/firewall/filter".
	Path string
	// Ordered is whether position carries meaning. It should come from the
	// IR's class rather than from a guess: a first-match chain is ordered, an
	// address list is not, and reordering the second is pointless churn.
	Ordered bool
	// Unlisted is required. See the type.
	Unlisted Unlisted
	// Key is a field the router itself enforces unique, or empty.
	//
	// It must come from schema.Identity.Tested, meaning a probe watched the
	// device refuse a second row and name this field — never from a field whose
	// name merely looks like an identifier. With a key, a desired row is matched
	// by it and everything else about the row can be updated in place. Without
	// one, matching falls back to the fields the spec sets, which cannot
	// distinguish "this row changed" from "this is a different row", so a change
	// becomes a delete and a create.
	Key string
}

// Op is what a step does.
type Op string

const (
	OpCreate Op = "create"
	OpUpdate Op = "update"
	OpDelete Op = "delete"
	OpMove   Op = "move"
)

// Step is one operation in a plan.
type Step struct {
	Op Op
	// ID is the row being acted on, for update, delete and move.
	ID string
	// Row is the body to send, for create and update. For update it holds only
	// the fields that differ.
	Row Record
	// Order is the rows to place, in the order they should end up, for move.
	Order []string
	// Before is the row they are placed in front of, or empty for the end of
	// the menu.
	Before string
	// Why is a short reason, for logs and for a dry run a human reads.
	Why string
}

// Plan is what Apply would do. It is computed without writing anything, so a
// caller can show it, refuse it, or count it.
type Plan struct {
	Steps []Step
	// Matched pairs a desired row's index with the device row it resolved to,
	// so a caller can report per-row status from a single object.
	Matched map[int]string
}

// Empty reports that the device already matches the spec.
func (p Plan) Empty() bool { return len(p.Steps) == 0 }

// Counts summarises a plan by operation, for a log line that does not need the
// whole thing.
func (p Plan) Counts() map[Op]int {
	out := map[Op]int{}
	for _, s := range p.Steps {
		out[s.Op]++
	}
	return out
}

// AmbiguousError is a desired row that matched more than one device row.
//
// It is an error rather than a choice on purpose. Picking one would silently
// manage an arbitrary row and leave its twin drifting, and the twin is usually
// somebody's hand-added configuration. Without a device-enforced key this is
// reachable on any router a person has touched: keyprobe found no menu where
// RouterOS enforces comment uniqueness, so matching on a comment can genuinely
// hit two rows.
//
// Candidates that differ in nothing but their id are not an error — the choice
// between them cannot be observed, so they are paired off in listed order.
type AmbiguousError struct {
	Path  string
	Index int
	Row   Record
	IDs   []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%s: desired row %d matches %d device rows (%s); "+
		"no field here is device-enforced unique, so this cannot be resolved by guessing",
		e.Path, e.Index, len(e.IDs), strings.Join(e.IDs, " "))
}

// ErrNoUnlistedPolicy is returned for a spec that has not chosen one.
var ErrNoUnlistedPolicy = errors.New("rest: MenuSpec.Unlisted must be set to tolerate or prune")

// Plan computes what it would take to make the menu match desired.
//
// Nothing is written. The device is read once, and the plan is a pure function
// of that reading and the spec, which is what makes it testable without a
// router and reviewable before it runs.
func (c *Client) Plan(ctx context.Context, spec MenuSpec, desired []Record) (Plan, error) {
	current, err := c.List(ctx, spec.Path)
	if err != nil {
		return Plan{}, err
	}
	return plan(spec, desired, current)
}

// plan is Plan without the I/O.
func plan(spec MenuSpec, desired, current []Record) (Plan, error) {
	switch spec.Unlisted {
	case UnlistedTolerate, UnlistedPrune:
	default:
		return Plan{}, ErrNoUnlistedPolicy
	}

	p := Plan{Matched: map[int]string{}}
	claimed := map[string]bool{}

	// Resolve every desired row first, so an ambiguity fails the whole plan
	// rather than after some of it has been applied.
	for i, want := range desired {
		ids := candidates(spec, want, current, claimed)
		// More than one candidate is only undecidable when the choice is
		// observable. Rows that are identical apart from their id can be paired
		// off in listed order: whichever is picked, the result is the same, and
		// N desired rows against N identical device rows is an exact match
		// rather than a conflict. Rows that differ in fields the spec does not
		// set are a different matter — picking one would manage it and leave
		// its twin drifting, and the twin is usually somebody's hand-added
		// configuration.
		if len(ids) > 1 && !indistinguishable(current, ids) {
			return Plan{}, &AmbiguousError{Path: spec.Path, Index: i, Row: want, IDs: ids}
		}
		if len(ids) > 0 {
			claimed[ids[0]] = true
			p.Matched[i] = ids[0]
		}
	}

	// Creates and updates, in desired order so the plan reads top to bottom.
	for i, want := range desired {
		id, ok := p.Matched[i]
		if !ok {
			p.Steps = append(p.Steps, Step{Op: OpCreate, Row: want, Why: "no device row matches"})
			continue
		}
		if spec.Key == "" {
			// Matched on the full set of specified fields, so there is nothing
			// left that could differ.
			continue
		}
		row := byID(current, id)
		if diff := changes(want, row); len(diff) > 0 {
			p.Steps = append(p.Steps, Step{Op: OpUpdate, ID: id, Row: diff,
				Why: "differs in " + strings.Join(slices.Sorted(maps.Keys(diff)), ", ")})
		}
	}

	if spec.Unlisted == UnlistedPrune {
		for _, row := range current {
			if id := row.ID(); id != "" && !claimed[id] {
				p.Steps = append(p.Steps, Step{Op: OpDelete, ID: id, Why: "not in the spec, and policy is prune"})
			}
		}
	}

	if spec.Ordered {
		p.Steps = append(p.Steps, reorder(spec, desired, current, p.Matched, claimed)...)
	}
	return p, nil
}

// candidates returns the device rows a desired row could be, excluding any
// already claimed by an earlier desired row.
func candidates(spec MenuSpec, want Record, current []Record, claimed map[string]bool) []string {
	var ids []string
	for _, row := range current {
		id := row.ID()
		if id == "" || claimed[id] {
			continue
		}
		if spec.Key != "" {
			// A device-enforced key cannot match twice, so this loop returns at
			// most one id and an ambiguity is impossible by construction.
			if v, ok := want[spec.Key]; ok {
				if got, present := row[spec.Key]; present && got == v {
					return []string{id}
				}
			}
			continue
		}
		if subset(want, row) {
			ids = append(ids, id)
		}
	}
	return ids
}

// indistinguishable reports that the candidate rows differ in nothing but their
// id, so which one is chosen cannot be observed.
func indistinguishable(current []Record, ids []string) bool {
	first := byID(current, ids[0])
	for _, id := range ids[1:] {
		other := byID(current, id)
		for k, v := range first {
			if k == IDField {
				continue
			}
			if got, present := other[k]; !present || got != v {
				return false
			}
		}
		for k := range other {
			if k == IDField {
				continue
			}
			if _, present := first[k]; !present {
				return false
			}
		}
	}
	return true
}

// subset reports that every field the spec sets already holds that value on the
// device row. Fields the spec does not mention are unmanaged and ignored, which
// is what lets a partial spec coexist with settings nobody wants to declare.
func subset(want, row Record) bool {
	for k, v := range want {
		if k == IDField {
			continue
		}
		if got, present := row[k]; !present || got != v {
			return false
		}
	}
	return true
}

// changes returns the fields of want that the device disagrees with.
func changes(want, row Record) Record {
	out := Record{}
	for k, v := range want {
		if k == IDField {
			continue
		}
		if got, present := row[k]; !present || got != v {
			out[k] = v
		}
	}
	return out
}

// reorder emits the moves that put the matched rows into desired order.
//
// Two shapes, because the cheap one is only correct when the resource owns the
// whole menu. RouterOS `move` takes numbers as a comma list and moves them as a
// block preserving their relative order, and omitting destination means the end
// of the menu — both verified against 7.23.2. So when every row is managed, one
// call sorts the menu. When rows are tolerated, moving the managed block would
// also reposition it relative to rows nobody asked to touch, which changes
// first-match behaviour for the unmanaged rules; that case walks backwards
// anchoring each row against its successor instead, leaving unmanaged rows where
// they are.
//
// Creates are not in the plan's id space yet, so ordering is only emitted for
// rows that already exist. Apply handles that boundary by executing structural
// steps first, reading the menu again, and replacing this preliminary ordering
// phase with one that includes the newly assigned ids.
func reorder(spec MenuSpec, desired, current []Record, matched map[int]string, claimed map[string]bool) []Step {
	// The managed rows, in the order the spec wants them.
	var want []string
	for i := range desired {
		if id, ok := matched[i]; ok {
			want = append(want, id)
		}
	}
	if len(want) < 2 {
		return nil
	}

	// The same rows in the order the device currently holds them.
	var have []string
	for _, row := range current {
		if id := row.ID(); claimed[id] {
			have = append(have, id)
		}
	}
	if slices.Equal(want, have) {
		return nil
	}

	if len(have) == len(current) {
		return []Step{{Op: OpMove, Order: want, Why: "the resource owns every row, so one move sorts the menu"}}
	}

	// Anchored pairwise: place each row immediately before the one that should
	// follow it, last pair first, so an earlier move cannot undo a later one.
	var steps []Step
	for i := len(want) - 2; i >= 0; i-- {
		steps = append(steps, Step{Op: OpMove, Order: []string{want[i]}, Before: want[i+1],
			Why: "unmanaged rows are tolerated here, so only the managed order is corrected"})
	}
	return steps
}

// Apply runs a plan. It re-plans first, so the reading the plan was computed
// from is as fresh as it can be.
//
// Steps run in plan order and the first failure stops it: a partially applied
// menu is visible in the next Plan, whereas continuing past an error can leave
// order half-corrected, which for a first-match chain is worse than not
// starting. The plan that was executed is returned so a caller can report what
// happened.
func (c *Client) Apply(ctx context.Context, spec MenuSpec, desired []Record) (Plan, error) {
	p, err := c.Plan(ctx, spec, desired)
	if err != nil {
		return Plan{}, err
	}

	// A move planned against the first reading cannot include rows created by
	// this apply, and deletes can make its anchors unnecessarily stale. Run the
	// structural phase first and plan order again from the resulting device
	// state, so a new first-match rule reaches its requested position before
	// this call returns.
	structural := spec.Ordered && slices.ContainsFunc(p.Steps, func(s Step) bool {
		return s.Op == OpCreate || s.Op == OpDelete
	})
	if structural {
		var mutations []Step
		for _, s := range p.Steps {
			if s.Op != OpMove {
				mutations = append(mutations, s)
			}
		}
		p.Steps = mutations
		for i, s := range mutations {
			if err := c.step(ctx, spec, s); err != nil {
				return p, fmt.Errorf("%s mutation step %d/%d (%s): %w", spec.Path, i+1, len(mutations), s.Op, err)
			}
		}

		ordered, err := c.Plan(ctx, spec, desired)
		if err != nil {
			return p, fmt.Errorf("%s: planning order after structural steps: %w", spec.Path, err)
		}
		for _, s := range ordered.Steps {
			if s.Op != OpMove {
				return p, fmt.Errorf("%s: structural steps did not converge; next plan still contains %s", spec.Path, s.Op)
			}
		}
		p.Matched = ordered.Matched
		p.Steps = append(p.Steps, ordered.Steps...)
		for i, s := range ordered.Steps {
			if err := c.step(ctx, spec, s); err != nil {
				return p, fmt.Errorf("%s ordering step %d/%d (%s): %w", spec.Path, i+1, len(ordered.Steps), s.Op, err)
			}
		}
		return p, nil
	}

	for i, s := range p.Steps {
		if err := c.step(ctx, spec, s); err != nil {
			return p, fmt.Errorf("%s step %d/%d (%s): %w", spec.Path, i+1, len(p.Steps), s.Op, err)
		}
	}
	return p, nil
}

func (c *Client) step(ctx context.Context, spec MenuSpec, s Step) error {
	switch s.Op {
	case OpCreate:
		_, err := c.Create(ctx, spec.Path, s.Row)
		return err
	case OpUpdate:
		_, err := c.Update(ctx, spec.Path, s.ID, s.Row)
		return err
	case OpDelete:
		return c.Delete(ctx, spec.Path, s.ID)
	case OpMove:
		args := Record{"numbers": strings.Join(s.Order, ",")}
		if s.Before != "" {
			args["destination"] = s.Before
		}
		_, err := c.Command(ctx, spec.Path, "move", args)
		return err
	default:
		return fmt.Errorf("rest: unknown step %q", s.Op)
	}
}

func byID(rows []Record, id string) Record {
	for _, r := range rows {
		if r.ID() == id {
			return r
		}
	}
	return nil
}
