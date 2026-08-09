package config

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

const ndPath = "/ipv6/nd"

// TestInterfaceIdentityLiveCHRNeighborDiscovery proves the adoption against a
// real router. A fresh RouterOS ships one /ipv6/nd row -- default=true,
// interface=all -- that cannot be added to and cannot be removed, so upstream
// create could only ever collide with it and the menu had to stay outside
// Crossplane. Create must take the row over in place, and delete must leave it
// standing.
func TestInterfaceIdentityLiveCHRNeighborDiscovery(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	res := providerForRuntime().ResourcesMap["routeros_ipv6_neighbor_discovery"]
	ctx := context.Background()

	// Confirm the premise: the router really does ship the undeletable row, so
	// the test would fail loudly if the fixture ever changed.
	rows, err := itemsByField(client, ndPath, defaultField, routerTrue)
	if err != nil {
		t.Fatalf("list default rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("router has %d default rows, want the single shipped one", len(rows))
	}
	defaultID := rows[0].GetID(routeros.Id)
	defaultInterface := rows[0][interfaceField]
	t.Cleanup(func() {
		_, _ = routeros.UpdateItem(&routeros.ItemId{Type: routeros.Id, Value: defaultID}, ndPath,
			routeros.MikrotikItem{ndLifetimeREST: ndShippedLifetime}, client)
	})

	// Adopt: create against the interface the default row occupies.
	adopted := natData(t, res, map[string]string{interfaceField: defaultInterface, ndLifetimeField: ndTestLifetime})
	if dg := res.CreateContext(ctx, adopted, client); dg.HasError() {
		t.Fatalf("create over the default row: %v", dg)
	}
	if adopted.Id() != defaultInterface {
		t.Errorf("id = %q, want the interface %q", adopted.Id(), defaultInterface)
	}

	after, err := itemsByField(client, ndPath, interfaceField, defaultInterface)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("interface %q carries %d rows, want 1 -- create added instead of adopting", defaultInterface, len(after))
	}
	if got := after[0].GetID(routeros.Id); got != defaultID {
		t.Errorf("adopted row id = %s, want the original default row %s", got, defaultID)
	}
	if got := after[0][ndLifetimeREST]; got != ndTestLifetime {
		t.Errorf("%s = %q, want the adopted %s", ndLifetimeREST, got, ndTestLifetime)
	}

	// Release: the router refuses to remove this row, so delete must drop it
	// from state and leave it alone.
	dg := res.DeleteContext(ctx, adopted, client)
	if dg.HasError() {
		t.Fatalf("delete the adopted default row: %v", dg)
	}
	if len(dg) != 1 || dg[0].Severity != diag.Warning {
		t.Errorf("diagnostics = %v, want a single warning that the row was left in place", dg)
	}
	survived, err := itemsByField(client, ndPath, defaultField, routerTrue)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(survived) != 1 {
		t.Fatalf("router has %d default rows after delete, want it untouched", len(survived))
	}

	// An ordinary row keeps ordinary semantics on the same menu.
	ordinary := natData(t, res, map[string]string{interfaceField: ifaceEther2})
	if dg := res.CreateContext(ctx, ordinary, client); dg.HasError() {
		t.Fatalf("create ether2: %v", dg)
	}
	if ordinary.Id() != ifaceEther2 {
		t.Errorf("id = %q, want %q", ordinary.Id(), ifaceEther2)
	}
	if dg := res.DeleteContext(ctx, ordinary, client); dg.HasError() {
		t.Fatalf("delete ether2: %v", dg)
	}
	gone, err := itemsByField(client, ndPath, interfaceField, ifaceEther2)
	if err != nil {
		t.Fatalf("list after ether2 delete: %v", err)
	}
	if len(gone) != 0 {
		t.Errorf("ether2 row survived its delete: %v", gone)
	}
}
