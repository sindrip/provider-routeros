package config

import (
	"context"
	"os"
	"testing"

	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// TestCommentIdentityLiveCHR exercises the comment-identity CRUD against a
// real RouterOS instance (hack/chr/run.sh). Run with:
//
//	CHR_REST=http://127.0.0.1:18080 go test -run LiveCHR ./config/
const (
	liveEther = "ether3"
	attrName  = "name"
)

func TestCommentIdentityLiveCHR(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	res := providerForRuntime().ResourcesMap["routeros_ip_firewall_nat"]
	ctx := context.Background()

	// A comment that stresses the filter encoding: spaces, %, &.
	comment := "ci live [50% off] & spaces"

	d := natData(t, res, map[string]string{attrChain: chainSrcnat, attrAction: actionMasquerade, commentField: comment})
	if dg := res.CreateContext(ctx, d, client); dg.HasError() {
		t.Fatalf("create: %v", dg)
	}
	if d.Id() != comment {
		t.Fatalf("create set id %q, want the comment", d.Id())
	}
	defer func() {
		del := natData(t, res, map[string]string{})
		del.SetId(comment)
		if dg := res.DeleteContext(ctx, del, client); dg.HasError() {
			t.Errorf("cleanup delete: %v", dg)
		}
	}()

	rd := natData(t, res, map[string]string{})
	rd.SetId(comment)
	if dg := res.ReadContext(ctx, rd, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if rd.Id() != comment {
		t.Fatalf("read left id %q", rd.Id())
	}
	if rd.Get(attrAction).(string) != actionMasquerade {
		t.Fatalf("read did not populate action: %q", rd.Get(attrAction))
	}

	dup := natData(t, res, map[string]string{attrChain: chainSrcnat, attrAction: actionMasquerade, commentField: comment})
	if dg := res.CreateContext(ctx, dup, client); !dg.HasError() {
		t.Fatal("duplicate-comment create succeeded on live router")
	}
}

func TestCommentIdentityLiveCHRBridgePort(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	res := providerForRuntime().ResourcesMap["routeros_interface_bridge_port"]
	ctx := context.Background()

	bridge, err := routeros.CreateItem(ctx, routeros.MikrotikItem{attrName: "ci-live-br"}, "/interface/bridge", client)
	if err != nil {
		t.Fatalf("bridge fixture: %v", err)
	}
	defer routeros.DeleteItem(&routeros.ItemId{Type: routeros.Id, Value: bridge.GetID(routeros.Id)}, "/interface/bridge", client) //nolint:errcheck

	comment := "ci live port [x] & y"
	d := natData(t, res, map[string]string{"bridge": "ci-live-br", "interface": liveEther, commentField: comment})
	if dg := res.CreateContext(ctx, d, client); dg.HasError() {
		t.Fatalf("create: %v", dg)
	}
	if d.Id() != comment {
		t.Fatalf("create set id %q, want the comment", d.Id())
	}

	rd := natData(t, res, map[string]string{})
	rd.SetId(comment)
	if dg := res.ReadContext(ctx, rd, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if rd.Get("interface").(string) != liveEther {
		t.Fatalf("read did not populate interface: %q", rd.Get("interface"))
	}

	del := natData(t, res, map[string]string{})
	del.SetId(comment)
	if dg := res.DeleteContext(ctx, del, client); dg.HasError() {
		t.Fatalf("delete: %v", dg)
	}
}

func TestFactoryIdentityLiveCHREthernet(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	res := providerForRuntime().ResourcesMap["routeros_interface_ethernet"]
	ctx := context.Background()

	// Adoption of the physical port by its immutable factory name.
	d := natData(t, res, map[string]string{"factory_name": liveEther, attrName: liveEther})
	if dg := res.CreateContext(ctx, d, client); dg.HasError() {
		t.Fatalf("adopt: %v", dg)
	}
	if d.Id() != liveEther {
		t.Fatalf("adopt set id %q, want the factory name", d.Id())
	}

	rd := natData(t, res, map[string]string{"factory_name": liveEther, attrName: liveEther})
	rd.SetId(liveEther)
	if dg := res.ReadContext(ctx, rd, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if rd.Id() != liveEther {
		t.Fatalf("read left id %q, want the factory name", rd.Id())
	}
	if rd.Get(attrName).(string) == "" {
		t.Fatal("read did not populate the interface name")
	}

	// A factory name that matches no hardware clears state on read.
	gone := natData(t, res, map[string]string{})
	gone.SetId("ether99")
	if dg := res.ReadContext(ctx, gone, client); dg.HasError() {
		t.Fatalf("read of missing port errored: %v", dg)
	}
	if gone.Id() != "" {
		t.Fatalf("read of missing port kept id %q, want cleared", gone.Id())
	}
}

func TestCommentIdentityLiveCHRDNSRecord(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	res := providerForRuntime().ResourcesMap["routeros_ip_dns_record"]
	ctx := context.Background()

	// Round-robin is why DNS records cannot use name identity: two records
	// with the SAME name must remain individually identifiable by comment.
	recs := []struct{ comment, address string }{
		{"ci flux ingress", "10.0.99.11"},
		{"ci flux ingress backup", "10.0.99.12"},
	}
	for _, r := range recs {
		d := natData(t, res, map[string]string{attrName: "ci.internal", "type": "A", "address": r.address, commentField: r.comment})
		if dg := res.CreateContext(ctx, d, client); dg.HasError() {
			t.Fatalf("create %q: %v", r.comment, dg)
		}
		if d.Id() != r.comment {
			t.Fatalf("create set id %q, want the comment", d.Id())
		}
	}
	defer func() {
		for _, r := range recs {
			del := natData(t, res, map[string]string{})
			del.SetId(r.comment)
			if dg := res.DeleteContext(ctx, del, client); dg.HasError() {
				t.Errorf("cleanup delete %q: %v", r.comment, dg)
			}
		}
	}()

	for _, r := range recs {
		rd := natData(t, res, map[string]string{})
		rd.SetId(r.comment)
		if dg := res.ReadContext(ctx, rd, client); dg.HasError() {
			t.Fatalf("read %q: %v", r.comment, dg)
		}
		if rd.Get("address").(string) != r.address {
			t.Fatalf("read %q resolved to address %q, want %q", r.comment, rd.Get("address"), r.address)
		}
	}
}

func TestCommentIdentityLiveCHRInterfaceListMember(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	res := providerForRuntime().ResourcesMap["routeros_interface_list_member"]
	ctx := context.Background()

	list, err := routeros.CreateItem(ctx, routeros.MikrotikItem{attrName: "ci-live-list"}, "/interface/list", client)
	if err != nil {
		t.Fatalf("list fixture: %v", err)
	}
	defer routeros.DeleteItem(&routeros.ItemId{Type: routeros.Id, Value: list.GetID(routeros.Id)}, "/interface/list", client) //nolint:errcheck

	comment := "ci live member [lan] & mgmt"
	d := natData(t, res, map[string]string{"list": "ci-live-list", "interface": liveEther, commentField: comment})
	if dg := res.CreateContext(ctx, d, client); dg.HasError() {
		t.Fatalf("create: %v", dg)
	}
	if d.Id() != comment {
		t.Fatalf("create set id %q, want the comment", d.Id())
	}

	rd := natData(t, res, map[string]string{})
	rd.SetId(comment)
	if dg := res.ReadContext(ctx, rd, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if rd.Get("interface").(string) != liveEther {
		t.Fatalf("read did not populate interface: %q", rd.Get("interface"))
	}

	del := natData(t, res, map[string]string{})
	del.SetId(comment)
	if dg := res.DeleteContext(ctx, del, client); dg.HasError() {
		t.Fatalf("delete: %v", dg)
	}
}

func TestCommentIdentityLiveCHRDhcpServerLease(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	res := providerForRuntime().ResourcesMap["routeros_ip_dhcp_server_lease"]
	ctx := context.Background()

	// A static lease row stands alone: no DHCP server needs to exist.
	comment := "ci live lease [jetkvm] & mgmt"
	d := natData(t, res, map[string]string{"address": "10.0.99.10", "mac_address": "30:52:53:08:3A:93", commentField: comment})
	if dg := res.CreateContext(ctx, d, client); dg.HasError() {
		t.Fatalf("create: %v", dg)
	}
	if d.Id() != comment {
		t.Fatalf("create set id %q, want the comment", d.Id())
	}

	rd := natData(t, res, map[string]string{})
	rd.SetId(comment)
	if dg := res.ReadContext(ctx, rd, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if rd.Get("address").(string) != "10.0.99.10" {
		t.Fatalf("read did not populate address: %q", rd.Get("address"))
	}

	del := natData(t, res, map[string]string{})
	del.SetId(comment)
	if dg := res.DeleteContext(ctx, del, client); dg.HasError() {
		t.Fatalf("delete: %v", dg)
	}
}

func TestCommentIdentityLiveCHRBridgeVlan(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	res := providerForRuntime().ResourcesMap["routeros_interface_bridge_vlan"]
	ctx := context.Background()

	bridge, err := routeros.CreateItem(ctx, routeros.MikrotikItem{attrName: "ci-live-vbr"}, "/interface/bridge", client)
	if err != nil {
		t.Fatalf("bridge fixture: %v", err)
	}
	defer routeros.DeleteItem(&routeros.ItemId{Type: routeros.Id, Value: bridge.GetID(routeros.Id)}, "/interface/bridge", client) //nolint:errcheck

	comment := "ci live vlan [30] & mgmt"
	d := natData(t, res, map[string]string{"bridge": "ci-live-vbr", "vlan_ids": "30", commentField: comment})
	if dg := res.CreateContext(ctx, d, client); dg.HasError() {
		t.Fatalf("create: %v", dg)
	}
	if d.Id() != comment {
		t.Fatalf("create set id %q, want the comment", d.Id())
	}

	rd := natData(t, res, map[string]string{})
	rd.SetId(comment)
	if dg := res.ReadContext(ctx, rd, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if rd.Get("bridge").(string) != "ci-live-vbr" {
		t.Fatalf("read did not populate bridge: %q", rd.Get("bridge"))
	}

	del := natData(t, res, map[string]string{})
	del.SetId(comment)
	if dg := res.DeleteContext(ctx, del, client); dg.HasError() {
		t.Fatalf("delete: %v", dg)
	}
}
