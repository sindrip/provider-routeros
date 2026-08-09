package config

import (
	"context"
	"os"
	"testing"

	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

func dropNDRow(client routeros.Client, iface string) {
	items, err := itemsByField(client, ndPath, interfaceField, iface)
	if err != nil {
		return
	}
	for _, it := range items {
		_ = routeros.DeleteItem(&routeros.ItemId{Type: routeros.Id, Value: it.GetID(routeros.Id)}, ndPath, client)
	}
}

// TestMistypedEnumsLiveCHRAdvertiseDNS proves the retype against a real
// router. /ipv6/nd advertise-dns is self|yes|no on 7.23 and rejects true and
// false, so the upstream bool could neither set self nor observe it: a device
// holding self read back as false, never matched the spec, and was overwritten
// with yes on every reconcile. As a string the value survives both directions.
func TestMistypedEnumsLiveCHRAdvertiseDNS(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	res := providerForRuntime().ResourcesMap["routeros_ipv6_neighbor_discovery"]
	ctx := context.Background()
	t.Cleanup(func() {
		dropNDRow(client, ifaceEther2)
		dropNDRow(client, ifaceEther3)
	})

	// Confirm the premise on this build: the router takes self and refuses the
	// boolean the upstream schema would have sent.
	probe, err := routeros.CreateItem(ctx, routeros.MikrotikItem{
		interfaceField: ifaceEther3, advertiseDNSREST: advertiseDNSSelf,
	}, ndPath, client)
	if err != nil {
		t.Fatalf("router rejected advertise-dns=self, so this build is not the one probed: %v", err)
	}
	if _, err := routeros.UpdateItem(&routeros.ItemId{Type: routeros.Id, Value: probe.GetID(routeros.Id)},
		ndPath, routeros.MikrotikItem{advertiseDNSREST: "true"}, client); err == nil {
		t.Fatal("router accepted advertise-dns=true; the field is a boolean after all and the retype is wrong")
	}
	dropNDRow(client, ifaceEther3)

	// self survives a create and reads back as itself.
	d := natData(t, res, map[string]string{
		interfaceField: ifaceEther2, advertiseDNSField: advertiseDNSSelf,
	})
	if dg := res.CreateContext(ctx, d, client); dg.HasError() {
		t.Fatalf("create with advertise_dns=self: %v", dg)
	}
	rows, err := itemsByField(client, ndPath, interfaceField, ifaceEther2)
	if err != nil || len(rows) != 1 {
		t.Fatalf("re-read: %v (%d rows)", err, len(rows))
	}
	if got := rows[0][advertiseDNSREST]; got != advertiseDNSSelf {
		t.Fatalf("router holds %s=%q, want %q", advertiseDNSREST, got, advertiseDNSSelf)
	}

	// The observation has to agree, or the reconciler writes over self forever.
	if dg := res.ReadContext(ctx, d, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if got := d.Get(advertiseDNSField).(string); got != advertiseDNSSelf {
		t.Fatalf("observed %s = %q, want %q -- observed never equals desired and the write loop returns",
			advertiseDNSField, got, advertiseDNSSelf)
	}

	// A spec that never mentions advertise-dns must not write one: the schema
	// default used to make every request carry yes.
	quiet := natData(t, res, map[string]string{interfaceField: ifaceEther3})
	if dg := res.CreateContext(ctx, quiet, client); dg.HasError() {
		t.Fatalf("create without advertise_dns: %v", dg)
	}
	untouched, err := itemsByField(client, ndPath, interfaceField, ifaceEther3)
	if err != nil || len(untouched) != 1 {
		t.Fatalf("re-read quiet row: %v (%d rows)", err, len(untouched))
	}
	if got := untouched[0][advertiseDNSREST]; got == "yes" {
		t.Errorf("create wrote %s=yes from a schema default the spec never asked for", advertiseDNSREST)
	}
}
