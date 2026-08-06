package config

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

const (
	testMenuPath = "/ip/firewall/filter"
	rowOne       = "one"
	rowTwo       = "two"
	rowThree     = "three"
)

// seqRouter emulates the REST surface the wrapped move CRUD touches: an
// order-preserving list and the /move command.
type seqRouter struct {
	order []map[string]string
	moves []string // recorded as "numbers>destination"
}

func (f *seqRouter) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest"+testMenuPath:
			json.NewEncoder(w).Encode(f.order)
		case r.Method == http.MethodPost && r.URL.Path == "/rest"+testMenuPath+"/move":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			f.moves = append(f.moves, body["numbers"]+">"+body["destination"])
			f.apply(strings.Split(body["numbers"], ","), body["destination"])
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
}

// apply reorders like RouterOS /move: the listed rows are removed and
// reinserted, in the order given, directly before the destination row.
func (f *seqRouter) apply(numbers []string, destination string) {
	moved := map[string]bool{}
	byID := map[string]map[string]string{}
	for _, it := range f.order {
		byID[it[attrID]] = it
	}
	pick := make([]map[string]string, 0, len(numbers))
	for _, n := range numbers {
		if it, ok := byID[n]; ok {
			moved[n] = true
			pick = append(pick, it)
		}
	}
	out := make([]map[string]string, 0, len(f.order))
	for _, it := range f.order {
		if moved[it[attrID]] {
			continue
		}
		if it[attrID] == destination {
			out = append(out, pick...)
		}
		out = append(out, it)
	}
	f.order = out
}

func (f *seqRouter) commentOrder() []string {
	out := make([]string, 0, len(f.order))
	for _, it := range f.order {
		out = append(out, it[commentField])
	}
	return out
}

func seqHarness(t *testing.T, order ...string) (*seqRouter, *schema.Resource, routeros.Client) {
	t.Helper()
	router := &seqRouter{}
	for i, comment := range order {
		router.order = append(router.order, map[string]string{attrID: "*" + strconv.Itoa(i+1), commentField: comment})
	}
	srv := httptest.NewServer(router.handler())
	t.Cleanup(srv.Close)
	res := providerForRuntime().ResourcesMap[moveItemsResource]
	return router, res, testClient(t, srv.URL)
}

// moveData builds a ResourceData for the move resource; natData cannot carry
// the list-valued sequence.
func moveData(t *testing.T, res *schema.Resource, path string, seq []string) *schema.ResourceData {
	t.Helper()
	attrs := map[string]string{resourcePathField: path, sequenceField + ".#": strconv.Itoa(len(seq))}
	seqVals := make([]cty.Value, 0, len(seq))
	for i, s := range seq {
		attrs[sequenceField+"."+strconv.Itoa(i)] = s
		seqVals = append(seqVals, cty.StringVal(s))
	}
	ctyVals := map[string]cty.Value{}
	for name, s := range res.Schema {
		switch {
		case name == resourcePathField:
			ctyVals[name] = cty.StringVal(path)
		case name == sequenceField && len(seq) > 0:
			ctyVals[name] = cty.ListVal(seqVals)
		case name == sequenceField:
			ctyVals[name] = cty.ListValEmpty(cty.String)
		default:
			ctyVals[name] = cty.NullVal(testCtyType(s))
		}
	}
	return res.Data(&terraform.InstanceState{Attributes: attrs, RawConfig: cty.ObjectVal(ctyVals)})
}

func stateSequence(d *schema.ResourceData) []string {
	raw := d.Get(sequenceField).([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(string))
	}
	return out
}

func TestCommentSequenceCreate(t *testing.T) {
	router, res, client := seqHarness(t, rowOne, rowTwo, rowThree)

	d := moveData(t, res, testMenuPath, []string{rowThree, rowOne, rowTwo})
	if dg := res.CreateContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("create: %v", dg)
	}
	if want := []string{"*3,*1>*2"}; !slices.Equal(router.moves, want) {
		t.Fatalf("move calls %v, want %v", router.moves, want)
	}
	if want := []string{rowThree, rowOne, rowTwo}; !slices.Equal(router.commentOrder(), want) {
		t.Fatalf("device order %v, want %v", router.commentOrder(), want)
	}
	if want := []string{rowThree, rowOne, rowTwo}; !slices.Equal(stateSequence(d), want) {
		t.Fatalf("state sequence %v, want the comments back", stateSequence(d))
	}
	// Upstream derives the id after appending the REST /move suffix; the
	// value is quirky but stable, which is all the external name needs.
	if d.Id() != "ip.firewall.filter.move" {
		t.Fatalf("create set id %q", d.Id())
	}
}

func TestCommentSequenceRawIDPassthrough(t *testing.T) {
	router, res, client := seqHarness(t, rowOne, rowTwo, rowThree)

	d := moveData(t, res, testMenuPath, []string{"*2", rowOne, rowThree})
	if dg := res.CreateContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("create: %v", dg)
	}
	if want := []string{rowTwo, rowOne, rowThree}; !slices.Equal(router.commentOrder(), want) {
		t.Fatalf("device order %v, want %v", router.commentOrder(), want)
	}
	if want := []string{"*2", rowOne, rowThree}; !slices.Equal(stateSequence(d), want) {
		t.Fatalf("state sequence %v, want raw id preserved", stateSequence(d))
	}
}

func TestCommentSequenceWriteErrors(t *testing.T) {
	_, res, client := seqHarness(t, rowOne, rowTwo, "dup", "dup")
	ctx := context.Background()

	for name, seq := range map[string][]string{
		"missing comment":   {rowOne, "ghost"},
		"ambiguous comment": {"dup", rowOne},
		"duplicate entry":   {rowOne, rowOne},
		"single entry":      {rowOne},
	} {
		d := moveData(t, res, testMenuPath, seq)
		if dg := res.CreateContext(ctx, d, client); !dg.HasError() {
			t.Errorf("%s: create succeeded, want error", name)
		}
	}
}

func TestCommentSequenceReadReportsDeviceOrder(t *testing.T) {
	_, res, client := seqHarness(t, rowTwo, rowOne, rowThree)

	d := moveData(t, res, testMenuPath, []string{rowOne, rowTwo, rowThree})
	d.SetId("ip.firewall.filter")
	if dg := res.ReadContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if want := []string{rowTwo, rowOne, rowThree}; !slices.Equal(stateSequence(d), want) {
		t.Fatalf("read sequence %v, want device order %v", stateSequence(d), want)
	}
}

func TestCommentSequenceReadDropsGoneRows(t *testing.T) {
	_, res, client := seqHarness(t, rowOne, rowThree)

	d := moveData(t, res, testMenuPath, []string{rowOne, "ghost", rowThree})
	d.SetId("ip.firewall.filter")
	if dg := res.ReadContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("read of sequence with gone row errored: %v", dg)
	}
	if want := []string{rowOne, rowThree}; !slices.Equal(stateSequence(d), want) {
		t.Fatalf("read sequence %v, want gone row dropped: %v", stateSequence(d), want)
	}
}
