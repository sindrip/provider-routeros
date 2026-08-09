package config

import (
	"context"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

const (
	ifaceAll    = "all"
	ifaceEther2 = "ether2"
	ifaceEther3 = "ether3"

	// ra-lifetime is a plain settable field, used here only to prove that a
	// spec value reaches the row the operation resolved to.
	ndLifetimeField   = "ra_lifetime"
	ndLifetimeREST    = "ra-lifetime"
	ndTestLifetime    = "20m"
	ndShippedLifetime = "30m"
)

// ndHarness serves /ipv6/nd preloaded with the router-owned default row, the
// one a fresh RouterOS always has and never lets go of.
func ndHarness(t *testing.T) (*fakeRouter, *schema.Resource, routeros.Client) {
	t.Helper()
	router := &fakeRouter{path: "/rest/ipv6/nd", nextID: 1, items: map[string]map[string]string{
		"*1": {attrID: "*1", interfaceField: ifaceAll, defaultField: routerTrue},
	}}
	srv := httptest.NewServer(router.handler())
	t.Cleanup(srv.Close)
	res := providerForRuntime().ResourcesMap["routeros_ipv6_neighbor_discovery"]
	return router, res, testClient(t, srv.URL)
}

// called reports whether the router saw a request with this method; a
// recorded call is "METHOD" for the collection and "METHOD id" for one row.
func called(router *fakeRouter, method string) bool {
	return slices.ContainsFunc(router.calls, func(c string) bool {
		return c == method || strings.HasPrefix(c, method+" ")
	})
}

func TestInterfaceIdentityResourceGates(t *testing.T) {
	for _, name := range interfaceIdentityResources {
		if slices.Contains(nameIdentityResources, name) || slices.Contains(commentIdentityResources, name) ||
			slices.Contains(factoryIdentityResources, name) {
			t.Errorf("%s is in multiple identity lists", name)
		}
		upstream := routeros.Provider().ResourcesMap[name]
		if upstream == nil {
			t.Fatalf("%s is not an upstream resource", name)
		}
		s := upstream.Schema[interfaceField]
		if s == nil || !s.Required {
			t.Errorf("%s must have a required %s field to serve as identity", name, interfaceField)
		}
		if got, _ := upstream.Schema[routeros.MetaId].Default.(int); got != int(routeros.Id) {
			t.Errorf("%s upstream ___id___ default = %v, want Id", name, got)
		}
	}
}

// TestInterfaceIdentityCreateAdoptsDefaultRow is the fix: the default row
// already occupies interface=all, so an add can only ever collide with it.
// Create must take the row over in place instead.
func TestInterfaceIdentityCreateAdoptsDefaultRow(t *testing.T) {
	router, res, client := ndHarness(t)
	d := natData(t, res, map[string]string{interfaceField: ifaceAll, ndLifetimeField: ndTestLifetime})

	if dg := res.CreateContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("create: %v", dg)
	}
	if d.Id() != ifaceAll {
		t.Errorf("id = %q, want %q", d.Id(), ifaceAll)
	}
	if called(router, "PUT") {
		t.Errorf("create added a row instead of adopting: %v", router.calls)
	}
	if got := router.items["*1"][ndLifetimeREST]; got != ndTestLifetime {
		t.Errorf("default row %s = %q, want the adopted value %s", ndLifetimeREST, got, ndTestLifetime)
	}
	if len(router.items) != 1 {
		t.Errorf("menu holds %d rows, want 1", len(router.items))
	}
}

func TestInterfaceIdentityCreateAddsWhenInterfaceIsFree(t *testing.T) {
	router, res, client := ndHarness(t)
	d := natData(t, res, map[string]string{interfaceField: ifaceEther2})

	if dg := res.CreateContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("create: %v", dg)
	}
	if d.Id() != ifaceEther2 {
		t.Errorf("id = %q, want %q", d.Id(), ifaceEther2)
	}
	if !called(router, "PUT") {
		t.Errorf("create did not add a row: %v", router.calls)
	}
	if len(router.items) != 2 {
		t.Errorf("menu holds %d rows, want 2", len(router.items))
	}
}

// A row somebody else placed on the interface is not ours to take over —
// only the router's own default row is.
func TestInterfaceIdentityCreateRejectsOccupiedInterface(t *testing.T) {
	router, res, client := ndHarness(t)
	router.items["*7"] = map[string]string{attrID: "*7", interfaceField: ifaceEther2, defaultField: "false"}
	d := natData(t, res, map[string]string{interfaceField: ifaceEther2})

	dg := res.CreateContext(context.Background(), d, client)
	if !dg.HasError() {
		t.Fatal("create adopted a row it does not own")
	}
	if !strings.Contains(dg[0].Summary, "already exists") {
		t.Errorf("diagnostic = %q, want it to name the collision", dg[0].Summary)
	}
	if called(router, "PUT") || called(router, "PATCH") {
		t.Errorf("create touched the router despite the collision: %v", router.calls)
	}
}

// Releasing the default row can only mean dropping it from state: the router
// answers "can not remove default rule" to a delete.
func TestInterfaceIdentityDeleteLeavesDefaultRow(t *testing.T) {
	router, res, client := ndHarness(t)
	d := natData(t, res, map[string]string{interfaceField: ifaceAll})
	d.SetId(ifaceAll)

	dg := res.DeleteContext(context.Background(), d, client)
	if dg.HasError() {
		t.Fatalf("delete: %v", dg)
	}
	if len(dg) != 1 || dg[0].Severity != diag.Warning {
		t.Errorf("diagnostics = %v, want a single warning", dg)
	}
	if called(router, "DELETE") {
		t.Errorf("delete tried to remove the default row: %v", router.calls)
	}
	if _, ok := router.items["*1"]; !ok {
		t.Error("default row disappeared from the router")
	}
	if d.Id() != "" {
		t.Errorf("id = %q, want it cleared", d.Id())
	}
}

func TestInterfaceIdentityDeleteRemovesOrdinaryRow(t *testing.T) {
	router, res, client := ndHarness(t)
	router.items["*7"] = map[string]string{attrID: "*7", interfaceField: ifaceEther2, defaultField: "false"}
	d := natData(t, res, map[string]string{interfaceField: ifaceEther2})
	d.SetId(ifaceEther2)

	if dg := res.DeleteContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("delete: %v", dg)
	}
	if _, ok := router.items["*7"]; ok {
		t.Error("row survived the delete")
	}
}

func TestInterfaceIdentityReadRestoresInterfaceAsID(t *testing.T) {
	router, res, client := ndHarness(t)
	router.items["*1"][ndLifetimeREST] = ndTestLifetime
	d := natData(t, res, map[string]string{interfaceField: ifaceAll})
	d.SetId(ifaceAll)

	if dg := res.ReadContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if d.Id() != ifaceAll {
		t.Errorf("id = %q, want the interface restored", d.Id())
	}
	if got := d.Get(ndLifetimeField).(string); got != ndTestLifetime {
		t.Errorf("%s = %q, want %s", ndLifetimeField, got, ndTestLifetime)
	}
}

func TestInterfaceIdentityReadClearsWhenRowIsGone(t *testing.T) {
	_, res, client := ndHarness(t)
	d := natData(t, res, map[string]string{interfaceField: ifaceEther3})
	d.SetId(ifaceEther3)

	if dg := res.ReadContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if d.Id() != "" {
		t.Errorf("id = %q, want it cleared for a vanished row", d.Id())
	}
}

// The router accepts moving a row to another interface, so the spec's
// interface can change — but not onto an interface already spoken for.
func TestInterfaceIdentityUpdateRejectsTakenInterface(t *testing.T) {
	router, res, client := ndHarness(t)
	router.items["*7"] = map[string]string{attrID: "*7", interfaceField: ifaceEther2, defaultField: "false"}
	d := natData(t, res, map[string]string{interfaceField: ifaceEther2})
	d.SetId(ifaceAll)

	dg := res.UpdateContext(context.Background(), d, client)
	if !dg.HasError() {
		t.Fatal("update moved a row onto an occupied interface")
	}
	if !strings.Contains(dg[0].Summary, "already exists") {
		t.Errorf("diagnostic = %q, want it to name the collision", dg[0].Summary)
	}
	if called(router, "PATCH") {
		t.Errorf("update patched despite the collision: %v", router.calls)
	}
}

func TestInterfaceIdentityUpdateMovesRowAndKeepsIdentity(t *testing.T) {
	router, res, client := ndHarness(t)
	d := natData(t, res, map[string]string{interfaceField: ifaceEther3})
	d.SetId(ifaceAll)

	if dg := res.UpdateContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("update: %v", dg)
	}
	if d.Id() != ifaceEther3 {
		t.Errorf("id = %q, want the new interface", d.Id())
	}
	if got := router.items["*1"][interfaceField]; got != ifaceEther3 {
		t.Errorf("router row interface = %q, want %q", got, ifaceEther3)
	}
}
