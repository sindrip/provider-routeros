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

	d := natData(t, res, map[string]string{"chain": "srcnat", "action": "masquerade", "comment": comment})
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
	if rd.Get("action").(string) != "masquerade" {
		t.Fatalf("read did not populate action: %q", rd.Get("action"))
	}

	dup := natData(t, res, map[string]string{"chain": "srcnat", "action": "masquerade", "comment": comment})
	if dg := res.CreateContext(ctx, dup, client); !dg.HasError() {
		t.Fatal("duplicate-comment create succeeded on live router")
	}
}
