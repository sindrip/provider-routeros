package config

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// TestPolicyExpansionLiveCHRUserGroup proves the loop-breaking read against a
// real router: a group created with three granted policies stores the full
// 17-member partition, and the wrapped read must collapse it back to the three
// the spec declares -- so observed equals desired and the reconciler no-ops.
func TestPolicyExpansionLiveCHRUserGroup(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	res := providerForRuntime().ResourcesMap["routeros_system_user_group"]
	ctx := context.Background()

	grp, err := routeros.CreateItem(ctx, routeros.MikrotikItem{
		attrName: "ci-live-ug", policyField: "read,api,rest-api",
	}, "/user/group", client)
	if err != nil {
		t.Fatalf("group fixture: %v", err)
	}
	defer routeros.DeleteItem(&routeros.ItemId{Type: routeros.Id, Value: grp.GetID(routeros.Id)}, "/user/group", client) //nolint:errcheck

	// Confirm the router really expanded the set, so the test would fail if the
	// collapse were a no-op.
	if got := len(strings.Split(grp[policyField], ",")); got < 10 {
		t.Fatalf("router stored %d policy members, expected the negated expansion", got)
	}

	rd := natData(t, res, map[string]string{})
	rd.SetId("ci-live-ug")
	if dg := res.ReadContext(ctx, rd, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}

	if got := policySet(rd); !slices.Equal(got, wantGranted) {
		t.Fatalf("observed policy = %v, want %v (negations collapsed)", got, wantGranted)
	}
}
