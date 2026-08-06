package config

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// TestCommentIdentityLiveCHR exercises the comment-identity CRUD against a
// real RouterOS instance (hack/chr/run.sh). Run with:
//
//	CHR_REST=http://127.0.0.1:18080 go test -run LiveCHR ./config/
func TestCommentIdentityLiveCHR(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	pd := schema.TestResourceDataRaw(t, routeros.Provider().Schema, map[string]any{
		"hosturl":          host,
		"username":         "admin",
		"routeros_version": "7.23.2",
	})
	c, dg := routeros.NewClient(context.Background(), pd)
	if dg.HasError() {
		t.Fatalf("NewClient: %v", dg)
	}
	client := c.(routeros.Client)
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

	pd := schema.TestResourceDataRaw(t, routeros.Provider().Schema, map[string]any{
		"hosturl":          host,
		"username":         "admin",
		"routeros_version": "7.23.2",
	})
	c, dg := routeros.NewClient(context.Background(), pd)
	if dg.HasError() {
		t.Fatalf("NewClient: %v", dg)
	}
	client := c.(routeros.Client)
	res := providerForRuntime().ResourcesMap["routeros_interface_bridge_port"]
	ctx := context.Background()

	bridge, err := routeros.CreateItem(ctx, routeros.MikrotikItem{"name": "ci-live-br"}, "/interface/bridge", client)
	if err != nil {
		t.Fatalf("bridge fixture: %v", err)
	}
	defer routeros.DeleteItem(&routeros.ItemId{Type: routeros.Id, Value: bridge.GetID(routeros.Id)}, "/interface/bridge", client) //nolint:errcheck

	comment := "ci live port [x] & y"
	d := natData(t, res, map[string]string{"bridge": "ci-live-br", "interface": "ether3", commentField: comment})
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
	if rd.Get("interface").(string) != "ether3" {
		t.Fatalf("read did not populate interface: %q", rd.Get("interface"))
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

	pd := schema.TestResourceDataRaw(t, routeros.Provider().Schema, map[string]any{
		"hosturl":          host,
		"username":         "admin",
		"routeros_version": "7.23.2",
	})
	c, dg := routeros.NewClient(context.Background(), pd)
	if dg.HasError() {
		t.Fatalf("NewClient: %v", dg)
	}
	client := c.(routeros.Client)
	res := providerForRuntime().ResourcesMap["routeros_interface_bridge_vlan"]
	ctx := context.Background()

	bridge, err := routeros.CreateItem(ctx, routeros.MikrotikItem{"name": "ci-live-vbr"}, "/interface/bridge", client)
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
