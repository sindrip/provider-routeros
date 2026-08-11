package rest

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// rows is shorthand for a device reading, with ids in listed order.
func rows(recs ...Record) []Record { return recs }

func rec(id string, kv ...string) Record {
	r := Record{IDField: id}
	for i := 0; i+1 < len(kv); i += 2 {
		r[kv[i]] = kv[i+1]
	}
	return r
}

func ops(p Plan) []Op {
	var out []Op
	for _, s := range p.Steps {
		out = append(out, s.Op)
	}
	return out
}

// orderingRouter is the small stateful slice of RouterOS needed to exercise
// Apply across its create, re-read and move phases. The planning tests above
// and below stay pure; this one exists because a created row's id is unknowable
// until the device returns it.
type orderingRouter struct {
	t      *testing.T
	rows   []Record
	nextID int
}

func (r *orderingRouter) client(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(r.serveHTTP))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, WithHTTPClient(srv.Client()), WithBasicAuth("admin", ""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func (r *orderingRouter) serveHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case req.Method == http.MethodGet && req.URL.Path == "/rest/ip/firewall/filter":
		writeJSON(r.t, w, http.StatusOK, r.rows)
	case req.Method == http.MethodPut && req.URL.Path == "/rest/ip/firewall/filter":
		var created Record
		r.decode(req, &created)
		r.nextID++
		created[IDField] = fmt.Sprintf("*%d", r.nextID)
		r.rows = append(r.rows, created)
		writeJSON(r.t, w, http.StatusCreated, created)
	case req.Method == http.MethodPost && req.URL.Path == "/rest/ip/firewall/filter/move":
		var args Record
		r.decode(req, &args)
		r.move(strings.Split(args["numbers"], ","), args["destination"])
		writeJSON(r.t, w, http.StatusOK, []Record{})
	default:
		r.t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
		http.Error(w, "unexpected request", http.StatusNotFound)
	}
}

func (r *orderingRouter) decode(req *http.Request, into any) {
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		r.t.Errorf("reading %s %s: %v", req.Method, req.URL.Path, err)
		return
	}
	if err := json.Unmarshal(raw, into); err != nil {
		r.t.Errorf("decoding %s %s: %v", req.Method, req.URL.Path, err)
	}
}

func (r *orderingRouter) move(ids []string, before string) {
	byID := map[string]Record{}
	selected := map[string]bool{}
	for _, row := range r.rows {
		byID[row.ID()] = row
	}
	for _, id := range ids {
		selected[id] = true
	}
	block := make([]Record, 0, len(ids))
	for _, id := range ids {
		block = append(block, byID[id])
	}
	remaining := slices.DeleteFunc(slices.Clone(r.rows), func(row Record) bool {
		return selected[row.ID()]
	})
	if before == "" {
		r.rows = append(remaining, block...)
		return
	}
	i := slices.IndexFunc(remaining, func(row Record) bool { return row.ID() == before })
	if i < 0 {
		r.t.Errorf("move destination %q is not present", before)
		return
	}
	out := append([]Record{}, remaining[:i]...)
	out = append(out, block...)
	r.rows = append(out, remaining[i:]...)
}

// TestUnlistedPolicyIsRequired is the gate ADR 0004 asks for. Prune deletes a
// person's hand-added configuration; tolerate never converges. A caller that has
// not chosen cannot be given either.
func TestUnlistedPolicyIsRequired(t *testing.T) {
	_, err := plan(MenuSpec{Path: "/ip/firewall/filter"}, nil, nil)
	if !errors.Is(err, ErrNoUnlistedPolicy) {
		t.Fatalf("a spec with no policy planned anyway: %v", err)
	}
	for _, policy := range []Unlisted{UnlistedTolerate, UnlistedPrune} {
		if _, err := plan(MenuSpec{Path: "/x", Unlisted: policy}, nil, nil); err != nil {
			t.Errorf("%s: %v", policy, err)
		}
	}
}

// TestToleratedRowsSurvive and TestPrunedRowsGo are the same input with the two
// policies, which is the clearest way to see what the choice actually costs.
func TestToleratedRowsSurvive(t *testing.T) {
	current := rows(
		rec("*1", "comment", "mine", "chain", "input"),
		rec("*2", "comment", "added-by-hand", "chain", "input"),
	)
	desired := []Record{{"comment": "mine", "chain": "input"}}

	p, err := plan(MenuSpec{Path: "/ip/firewall/filter", Unlisted: UnlistedTolerate}, desired, current)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Empty() {
		t.Errorf("nothing needed doing, got %v", ops(p))
	}
}

func TestPrunedRowsGo(t *testing.T) {
	current := rows(
		rec("*1", "comment", "mine", "chain", "input"),
		rec("*2", "comment", "added-by-hand", "chain", "input"),
	)
	desired := []Record{{"comment": "mine", "chain": "input"}}

	p, err := plan(MenuSpec{Path: "/ip/firewall/filter", Unlisted: UnlistedPrune}, desired, current)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ops(p), []Op{OpDelete}) {
		t.Fatalf("ops = %v, want one delete", ops(p))
	}
	if p.Steps[0].ID != "*2" {
		t.Errorf("deleted %s, want the unlisted row *2", p.Steps[0].ID)
	}
}

func TestIgnoredRowsAreNeitherMatchedNorPruned(t *testing.T) {
	current := rows(
		rec("*1", "chain", "input", "action", "accept", "dynamic", "true"),
		rec("*2", "chain", "input", "action", "drop"),
	)
	desired := []Record{{"chain": "input", "action": "accept"}}
	spec := MenuSpec{
		Path:     "/ip/firewall/filter",
		Ordered:  true,
		Unlisted: UnlistedPrune,
		Ignore:   []Record{{"dynamic": "true"}},
	}

	p, err := plan(spec, desired, current)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ops(p), []Op{OpCreate, OpDelete}) {
		t.Fatalf("ops = %v, want create for desired static row and delete for unlisted static row", ops(p))
	}
	if p.Steps[1].ID != "*2" {
		t.Fatalf("deleted %q, want static row *2", p.Steps[1].ID)
	}
	if _, matched := p.Matched[0]; matched {
		t.Fatal("desired static row adopted an ignored dynamic row")
	}
}

func TestEmptyIgnoreSelectorIsRejected(t *testing.T) {
	_, err := plan(MenuSpec{Path: "/x", Unlisted: UnlistedPrune, Ignore: []Record{{}}}, nil, nil)
	if !errors.Is(err, ErrEmptyIgnoreSelector) {
		t.Fatalf("error = %v, want ErrEmptyIgnoreSelector", err)
	}
}

// TestAmbiguityIsAnErrorNotAChoice is the consequence of keyprobe's finding that
// RouterOS enforces comment uniqueness nowhere. Two rows can share a comment, so
// a match can be genuinely undecidable, and picking one would manage an
// arbitrary row while its twin drifts.
//
// The rows below differ in an action the spec does not set, which is what makes
// the choice observable — the realistic shape of this is a hand-added rule that
// happens to carry a managed rule's comment. Candidates that differ in nothing
// at all are covered by TestIndistinguishableCandidatesArePairedOff.
func TestAmbiguityIsAnErrorNotAChoice(t *testing.T) {
	current := rows(
		rec("*1", "comment", "dup", "chain", "input", "action", "accept"),
		rec("*2", "comment", "dup", "chain", "input", "action", "drop"),
	)
	desired := []Record{{"comment": "dup", "chain": "input"}}

	_, err := plan(MenuSpec{Path: "/ip/firewall/filter", Unlisted: UnlistedTolerate}, desired, current)
	var amb *AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("err = %v, want AmbiguousError", err)
	}
	if len(amb.IDs) != 2 {
		t.Errorf("IDs = %v, want both rows", amb.IDs)
	}
	// And nothing is planned: a caller cannot half-apply an ambiguous menu.
	if amb.Index != 0 {
		t.Errorf("Index = %d, want the offending desired row", amb.Index)
	}
}

// TestIndistinguishableCandidatesArePairedOff is the other half of the rule: two
// device rows that differ in nothing but their id cannot be told apart by any
// caller either, so refusing to proceed would be pedantry rather than safety.
// Which one a desired row takes is unobservable; that two desired rows take two
// different ones is not, and is asserted.
func TestIndistinguishableCandidatesArePairedOff(t *testing.T) {
	current := rows(
		rec("*1", "comment", "dup", "chain", "input"),
		rec("*2", "comment", "dup", "chain", "input"),
	)

	one, err := plan(MenuSpec{Path: "/ip/firewall/filter", Unlisted: UnlistedTolerate},
		[]Record{{"comment": "dup", "chain": "input"}}, current)
	if err != nil {
		t.Fatalf("identical rows are not a conflict: %v", err)
	}
	if !one.Empty() {
		t.Errorf("ops = %v, want nothing", ops(one))
	}
	// Under prune the surplus identical row still goes, because the spec asked
	// for one row and the menu holds two.
	pruned, err := plan(MenuSpec{Path: "/ip/firewall/filter", Unlisted: UnlistedPrune},
		[]Record{{"comment": "dup", "chain": "input"}}, current)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ops(pruned), []Op{OpDelete}) {
		t.Errorf("ops = %v, want the surplus row deleted", ops(pruned))
	}
}

// TestADeviceKeyMakesUpdateInPlacePossible is the difference a proven key buys.
// Without one, "the comment changed" is indistinguishable from "this is a
// different row", so the only safe move is delete-and-create.
func TestADeviceKeyMakesUpdateInPlacePossible(t *testing.T) {
	current := rows(rec("*1", "name", "bridge0", "mtu", "1500", "comment", "old"))
	desired := []Record{{"name": "bridge0", "mtu": "9000", "comment": "new"}}

	withKey, err := plan(MenuSpec{Path: "/interface/bridge", Unlisted: UnlistedPrune, Key: "name"}, desired, current)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ops(withKey), []Op{OpUpdate}) {
		t.Fatalf("with a key: ops = %v, want one update", ops(withKey))
	}
	diff := withKey.Steps[0].Row
	if diff["mtu"] != "9000" || diff["comment"] != "new" {
		t.Errorf("update body = %v, want only the changed fields", diff)
	}
	if _, sent := diff["name"]; sent {
		t.Errorf("update body resends the unchanged key: %v", diff)
	}

	noKey, err := plan(MenuSpec{Path: "/interface/bridge", Unlisted: UnlistedPrune}, desired, current)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ops(noKey), []Op{OpCreate, OpDelete}) {
		t.Errorf("without a key: ops = %v, want create then delete", ops(noKey))
	}
}

// TestOwnedMenuSortsInOneMove pins the semantics verified against 7.23.2:
// numbers takes a comma list moved as a block preserving relative order, and no
// destination means the end of the menu.
func TestOwnedMenuSortsInOneMove(t *testing.T) {
	current := rows(
		rec("*1", "comment", "c"),
		rec("*2", "comment", "a"),
		rec("*3", "comment", "b"),
	)
	desired := []Record{{"comment": "a"}, {"comment": "b"}, {"comment": "c"}}

	p, err := plan(MenuSpec{Path: "/ip/firewall/filter", Ordered: true, Unlisted: UnlistedPrune}, desired, current)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ops(p), []Op{OpMove}) {
		t.Fatalf("ops = %v, want a single move", ops(p))
	}
	if got := p.Steps[0].Order; !slices.Equal(got, []string{"*2", "*3", "*1"}) {
		t.Errorf("Order = %v, want the ids in desired order", got)
	}
	if p.Steps[0].Before != "" {
		t.Errorf("Before = %q, want the end of the menu", p.Steps[0].Before)
	}
}

// TestApplyOrdersCreatedRowBeforeReturning covers the id boundary that Plan
// alone cannot: RouterOS assigns an id during create, and that id must take
// part in the ordering phase of the same Apply. Leaving the new rule at the end
// until another reconcile preserves the dead-rule window ADR 0004 removes.
func TestApplyOrdersCreatedRowBeforeReturning(t *testing.T) {
	router := &orderingRouter{
		t:      t,
		nextID: 2,
		rows: rows(
			rec("*1", "comment", "accept-first"),
			rec("*2", "comment", "drop-last"),
		),
	}
	c := router.client(t)
	spec := MenuSpec{Path: "/ip/firewall/filter", Ordered: true, Unlisted: UnlistedPrune}
	desired := []Record{
		{"comment": "accept-first"},
		{"comment": "accept-middle"},
		{"comment": "drop-last"},
	}

	p, err := c.Apply(t.Context(), spec, desired)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := p.Counts(); got[OpCreate] != 1 || got[OpMove] != 1 {
		t.Fatalf("operations = %v, want one create followed by one move", got)
	}
	got := make([]string, 0, len(router.rows))
	for _, row := range router.rows {
		got = append(got, row["comment"])
	}
	if want := []string{"accept-first", "accept-middle", "drop-last"}; !slices.Equal(got, want) {
		t.Errorf("order after Apply = %v, want %v", got, want)
	}

	converged, err := c.Apply(t.Context(), spec, desired)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if !converged.Empty() {
		t.Errorf("second Apply planned %v, want no work", converged.Counts())
	}
}

func TestApplyCheckedDoesNotMutateRejectedPlan(t *testing.T) {
	router := &orderingRouter{
		t:      t,
		nextID: 1,
		rows:   rows(rec("*1", "comment", "existing")),
	}
	c := router.client(t)
	spec := MenuSpec{Path: "/ip/firewall/filter", Ordered: true, Unlisted: UnlistedPrune}

	p, err := c.ApplyChecked(t.Context(), spec, nil, func(plan Plan) error {
		if !slices.Equal(ops(plan), []Op{OpDelete}) {
			t.Fatalf("approval plan ops = %v, want delete", ops(plan))
		}
		if plan.Steps[0].Row["comment"] != "existing" {
			t.Fatalf("delete preview row = %v", plan.Steps[0].Row)
		}
		return errors.New("not approved")
	})
	if err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("ApplyChecked() error = %v", err)
	}
	if !slices.Equal(ops(p), []Op{OpDelete}) {
		t.Fatalf("returned plan ops = %v", ops(p))
	}
	if len(router.rows) != 1 || router.rows[0].ID() != "*1" {
		t.Fatalf("router mutated after rejected plan: %v", router.rows)
	}
}

// TestToleratedReorderLeavesUnmanagedRowsAlone is why the one-call sort is not
// used unconditionally: moving the managed block would also move it relative to
// rules nobody asked to touch, which changes first-match behaviour for them.
func TestToleratedReorderLeavesUnmanagedRowsAlone(t *testing.T) {
	current := rows(
		rec("*1", "comment", "mine-b"),
		rec("*2", "comment", "theirs"),
		rec("*3", "comment", "mine-a"),
	)
	desired := []Record{{"comment": "mine-a"}, {"comment": "mine-b"}}

	p, err := plan(MenuSpec{Path: "/ip/firewall/filter", Ordered: true, Unlisted: UnlistedTolerate}, desired, current)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ops(p), []Op{OpMove}) {
		t.Fatalf("ops = %v, want anchored moves only", ops(p))
	}
	s := p.Steps[0]
	if !slices.Equal(s.Order, []string{"*3"}) || s.Before != "*1" {
		t.Errorf("move %v before %q, want *3 before *1", s.Order, s.Before)
	}
}

// TestUnorderedMenusAreNotReordered guards against pointless churn: an address
// list has no first-match semantics, so its order is not the provider's
// business.
func TestUnorderedMenusAreNotReordered(t *testing.T) {
	current := rows(rec("*1", "address", "10.0.0.2"), rec("*2", "address", "10.0.0.1"))
	desired := []Record{{"address": "10.0.0.1"}, {"address": "10.0.0.2"}}

	p, err := plan(MenuSpec{Path: "/ip/address", Unlisted: UnlistedPrune}, desired, current)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Empty() {
		t.Errorf("ops = %v, want nothing: order is meaningless here", ops(p))
	}
}

// TestAlreadyOrderedIsNoOp is the property that keeps a reconcile loop quiet.
func TestAlreadyOrderedIsNoOp(t *testing.T) {
	current := rows(rec("*1", "comment", "a"), rec("*2", "comment", "b"))
	desired := []Record{{"comment": "a"}, {"comment": "b"}}

	p, err := plan(MenuSpec{Path: "/ip/firewall/filter", Ordered: true, Unlisted: UnlistedPrune}, desired, current)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Empty() {
		t.Errorf("ops = %v, want nothing", ops(p))
	}
}

// TestEachDeviceRowIsClaimedOnce covers two desired rows that are identical in
// the fields they set. They must take two different device rows rather than both
// resolving to the first — and with only one row present, the second is a create.
func TestEachDeviceRowIsClaimedOnce(t *testing.T) {
	desired := []Record{{"chain": "input", "action": "accept"}, {"chain": "input", "action": "accept"}}

	twoRows := rows(rec("*1", "chain", "input", "action", "accept"), rec("*2", "chain", "input", "action", "accept"))
	p, err := plan(MenuSpec{Path: "/ip/firewall/filter", Unlisted: UnlistedPrune}, desired, twoRows)
	if err != nil {
		t.Fatalf("two identical rows for two identical specs is not ambiguous: %v", err)
	}
	if !p.Empty() {
		t.Errorf("ops = %v, want nothing", ops(p))
	}
	if p.Matched[0] == p.Matched[1] {
		t.Errorf("both desired rows claimed %s", p.Matched[0])
	}

	oneRow := rows(rec("*1", "chain", "input", "action", "accept"))
	p, err = plan(MenuSpec{Path: "/ip/firewall/filter", Unlisted: UnlistedPrune}, desired, oneRow)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ops(p), []Op{OpCreate}) {
		t.Errorf("ops = %v, want one create for the unmatched duplicate", ops(p))
	}
}

// TestUnmentionedFieldsAreNotManaged is what lets a partial spec coexist with
// settings nobody wants to declare. A row matching on everything the spec sets
// is a match, whatever else it carries.
func TestUnmentionedFieldsAreNotManaged(t *testing.T) {
	current := rows(rec("*1", "comment", "mine", "chain", "input", "log", "true", "bytes", "9001"))
	desired := []Record{{"comment": "mine", "chain": "input"}}

	p, err := plan(MenuSpec{Path: "/ip/firewall/filter", Unlisted: UnlistedPrune}, desired, current)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Empty() {
		t.Errorf("ops = %v; log and bytes are unmanaged and must not provoke anything", ops(p))
	}
}

// TestPresentEmptyIsNotAbsent carries Record's three-state boolean contract
// through menu reconciliation. RouterOS uses a present empty string for a set
// flag and omission for an unset one, so ordinary map zero-value comparisons
// silently reverse the intended state.
func TestPresentEmptyIsNotAbsent(t *testing.T) {
	t.Run("keyed row is updated", func(t *testing.T) {
		current := rows(rec("*1", "name", "peer"))
		desired := []Record{{"name": "peer", "ebgp": ""}}
		p, err := plan(MenuSpec{Path: "/routing/bgp/session", Unlisted: UnlistedTolerate, Key: "name"}, desired, current)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(ops(p), []Op{OpUpdate}) {
			t.Fatalf("ops = %v, want an update that sets the flag", ops(p))
		}
		if value, present := p.Steps[0].Row["ebgp"]; !present || value != "" {
			t.Errorf("update = %v, want ebgp present with an empty value", p.Steps[0].Row)
		}
	})

	t.Run("unkeyed row does not match", func(t *testing.T) {
		current := rows(rec("*1", "name", "peer"))
		desired := []Record{{"name": "peer", "ebgp": ""}}
		p, err := plan(MenuSpec{Path: "/routing/bgp/session", Unlisted: UnlistedPrune}, desired, current)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(ops(p), []Op{OpCreate, OpDelete}) {
			t.Errorf("ops = %v, want replace because the requested flag is absent", ops(p))
		}
	})

	t.Run("candidate rows remain distinguishable", func(t *testing.T) {
		current := rows(
			rec("*1", "name", "peer", "ebgp", ""),
			rec("*2", "name", "peer", "ibgp", ""),
		)
		_, err := plan(MenuSpec{Path: "/routing/bgp/session", Unlisted: UnlistedTolerate},
			[]Record{{"name": "peer"}}, current)
		var amb *AmbiguousError
		if !errors.As(err, &amb) {
			t.Fatalf("err = %v, want ambiguity between different set flags", err)
		}
	})

	t.Run("empty key does not match a missing key", func(t *testing.T) {
		got := candidates(MenuSpec{Key: "flag"}, Record{"flag": ""}, rows(rec("*1", "name", "row")), nil)
		if len(got) != 0 {
			t.Errorf("candidates = %v, want none", got)
		}
	})
}
