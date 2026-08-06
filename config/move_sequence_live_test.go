package config

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// TestCommentSequenceLiveCHRFirewallFilter exercises the comment-addressed
// sequencer against a real RouterOS instance: create rules by comment, order
// them with a move Items resource, verify the device order, then reorder
// through update.
func TestCommentSequenceLiveCHRFirewallFilter(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	rules := providerForRuntime().ResourcesMap["routeros_ip_firewall_filter"]
	move := providerForRuntime().ResourcesMap[moveItemsResource]
	ctx := context.Background()

	comments := []string{"ci seq [1] & a", "ci seq [2] & b", "ci seq [3] & c"}
	for _, c := range comments {
		d := natData(t, rules, map[string]string{attrChain: "ci-seq", attrAction: actionAccept, commentField: c})
		if dg := rules.CreateContext(ctx, d, client); dg.HasError() {
			t.Fatalf("rule create %q: %v", c, dg)
		}
	}
	defer func() {
		for _, c := range comments {
			del := natData(t, rules, map[string]string{})
			del.SetId(c)
			if dg := rules.DeleteContext(ctx, del, client); dg.HasError() {
				t.Errorf("cleanup delete %q: %v", c, dg)
			}
		}
	}()

	deviceOrder := func() []string {
		res, err := routeros.ReadItems(nil, "/ip/firewall/filter", client)
		if err != nil {
			t.Fatalf("list menu: %v", err)
		}
		var out []string
		for _, item := range *res {
			if slices.Contains(comments, item[commentField]) {
				out = append(out, item[commentField])
			}
		}
		return out
	}

	desired := []string{comments[2], comments[0], comments[1]}
	d := moveData(t, move, "/ip/firewall/filter", desired)
	if dg := move.CreateContext(ctx, d, client); dg.HasError() {
		t.Fatalf("move create: %v", dg)
	}
	if got := stateSequence(d); !slices.Equal(got, desired) {
		t.Fatalf("state sequence after create %v, want %v", got, desired)
	}
	if got := deviceOrder(); !slices.Equal(got, desired) {
		t.Fatalf("device order after create %v, want %v", got, desired)
	}

	// A clean read observes the applied order: no drift.
	rd := moveData(t, move, "/ip/firewall/filter", desired)
	rd.SetId(d.Id())
	if dg := move.ReadContext(ctx, rd, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if got := stateSequence(rd); !slices.Equal(got, desired) {
		t.Fatalf("read sequence %v, want %v", got, desired)
	}

	// Reorder through update.
	desired = []string{comments[1], comments[2], comments[0]}
	u := moveData(t, move, "/ip/firewall/filter", desired)
	u.SetId(d.Id())
	if dg := move.UpdateContext(ctx, u, client); dg.HasError() {
		t.Fatalf("move update: %v", dg)
	}
	if got := deviceOrder(); !slices.Equal(got, desired) {
		t.Fatalf("device order after update %v, want %v", got, desired)
	}
}
