package config

import (
	"context"
	"slices"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// storedPartition is the 17-member set RouterOS stores for policy=read,api,rest-api
// (CHR 7.23.2): the three granted keywords plus an explicit negation of every
// other keyword. wantGranted is what a loop-free read must leave behind.
var (
	storedPartition = []any{
		"read", "api", "rest-api",
		"!local", "!telnet", "!ssh", "!ftp", "!reboot", "!write", "!policy",
		"!test", "!winbox", "!password", "!web", "!sniff", "!sensitive", "!romon",
	}
	wantGranted = []string{"api", "read", "rest-api"}
)

func policySet(d *schema.ResourceData) []string {
	list := d.Get(policyField).(*schema.Set).List()
	out := make([]string, 0, len(list))
	for _, v := range list {
		out = append(out, v.(string))
	}
	sort.Strings(out)
	return out
}

func TestCollapsePolicyReadStripsNegations(t *testing.T) {
	res := providerForRuntime().ResourcesMap["routeros_system_user_group"]
	stub := func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		return diag.FromErr(d.Set(policyField, storedPartition))
	}

	d := res.TestResourceData()
	d.SetId("read")
	if dg := collapsePolicyRead(stub)(context.Background(), d, nil); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}

	if got := policySet(d); !slices.Equal(got, wantGranted) {
		t.Fatalf("policy = %v, want %v (negations stripped)", got, wantGranted)
	}
}

func TestCollapsePolicyReadIdempotentAndInert(t *testing.T) {
	res := providerForRuntime().ResourcesMap["routeros_system_user_group"]

	// A read that already returns only granted members must be left untouched,
	// and re-collapsing an already-collapsed set must not change it.
	positive := []any{"read", "api", "rest-api"}
	stub := func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		return diag.FromErr(d.Set(policyField, positive))
	}
	d := res.TestResourceData()
	d.SetId("read")
	collapse := collapsePolicyRead(stub)
	if dg := collapse(context.Background(), d, nil); dg.HasError() {
		t.Fatalf("first read: %v", dg)
	}
	if dg := collapse(context.Background(), d, nil); dg.HasError() {
		t.Fatalf("second read: %v", dg)
	}
	if got := policySet(d); !slices.Equal(got, wantGranted) {
		t.Fatalf("policy = %v, want %v", got, wantGranted)
	}
}

func TestCollapsePolicyReadSkipsDeleted(t *testing.T) {
	res := providerForRuntime().ResourcesMap["routeros_system_user_group"]
	// A read that clears the id (item gone) must short-circuit, not touch state.
	stub := func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		d.SetId("")
		return nil
	}
	d := res.TestResourceData()
	d.SetId("read")
	if dg := collapsePolicyRead(stub)(context.Background(), d, nil); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if d.Id() != "" {
		t.Fatalf("id = %q, want cleared", d.Id())
	}
}

func TestPolicyExpansionResourceGates(t *testing.T) {
	for _, name := range policyExpansionResources {
		up := routeros.Provider().ResourcesMap[name]
		if up == nil {
			t.Fatalf("%s is not an upstream resource", name)
		}
		if s := up.Schema[policyField]; s == nil || s.Type != schema.TypeSet {
			t.Errorf("%s has no %s set field to normalize", name, policyField)
		}
	}
}
