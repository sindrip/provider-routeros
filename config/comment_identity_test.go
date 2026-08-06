package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// fakeRouter emulates the REST surface the wrapped NAT CRUD touches:
// list-with-comment-filter, get/patch/delete by id, and create.
type fakeRouter struct {
	items  map[string]map[string]string // .id -> item
	nextID int
}

func (f *fakeRouter) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/ip/firewall/nat")
		id = strings.TrimPrefix(id, "/")
		switch {
		case r.Method == http.MethodGet && id == "":
			out := []map[string]string{}
			for _, it := range f.items {
				if c, filtered := r.URL.Query()["comment"]; filtered && it["comment"] != c[0] {
					continue
				}
				if c, filtered := r.URL.Query()[".id"]; filtered && it[".id"] != c[0] {
					continue
				}
				out = append(out, it)
			}
			json.NewEncoder(w).Encode(out)
		case r.Method == http.MethodGet:
			if it, ok := f.items[id]; ok {
				json.NewEncoder(w).Encode(it)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut:
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			f.nextID++
			body[".id"] = fmt.Sprintf("*%X", f.nextID)
			f.items[body[".id"]] = body
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(body)
		case r.Method == http.MethodPatch:
			it, ok := f.items[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			for k, v := range body {
				it[k] = v
			}
			json.NewEncoder(w).Encode(it)
		case r.Method == http.MethodDelete:
			delete(f.items, id)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
}

func natHarness(t *testing.T) (*fakeRouter, *schema.Resource, routeros.Client) {
	t.Helper()
	router := &fakeRouter{items: map[string]map[string]string{}}
	srv := httptest.NewServer(router.handler())
	t.Cleanup(srv.Close)
	// Construct through NewClient like the real provider: RestClient has
	// unexported fields (ctx, extra) that SendRequest dereferences.
	pd := schema.TestResourceDataRaw(t, routeros.Provider().Schema, map[string]any{
		"hosturl":          srv.URL,
		"username":         "admin",
		"routeros_version": "7.23.2",
	})
	c, dg := routeros.NewClient(context.Background(), pd)
	if dg.HasError() {
		t.Fatalf("NewClient: %v", dg)
	}
	client := c.(routeros.Client)
	res := providerForRuntime().ResourcesMap["routeros_ip_firewall_nat"]
	return router, res, client
}

// natData builds a ResourceData whose GetRawConfig works: the upstream
// serializer dereferences it per schema field, and the SDK's
// TestResourceDataRaw leaves it null. All test values are strings.
func natData(t *testing.T, res *schema.Resource, vals map[string]string) *schema.ResourceData {
	t.Helper()
	attrs := map[string]string{}
	ctyVals := map[string]cty.Value{}
	for name, s := range res.Schema {
		typ := testCtyType(s)
		if v, ok := vals[name]; ok {
			attrs[name] = v
			ctyVals[name] = cty.StringVal(v)
		} else {
			ctyVals[name] = cty.NullVal(typ)
		}
	}
	return res.Data(&terraform.InstanceState{Attributes: attrs, RawConfig: cty.ObjectVal(ctyVals)})
}

func testCtyType(s *schema.Schema) cty.Type {
	switch s.Type {
	case schema.TypeBool:
		return cty.Bool
	case schema.TypeInt, schema.TypeFloat:
		return cty.Number
	case schema.TypeList:
		return cty.List(cty.String)
	case schema.TypeSet:
		return cty.Set(cty.String)
	case schema.TypeMap:
		return cty.Map(cty.String)
	default:
		return cty.String
	}
}

func TestCommentIdentityCreate(t *testing.T) {
	router, res, client := natHarness(t)

	d := natData(t, res, map[string]string{"chain": "srcnat", "action": "masquerade"})
	if dg := res.CreateContext(context.Background(), d, client); !dg.HasError() {
		t.Fatal("create without comment succeeded, want identity error")
	}

	d = natData(t, res, map[string]string{"chain": "srcnat", "action": "masquerade", "comment": "wan masquerade"})
	if dg := res.CreateContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("create failed: %v", dg)
	}
	if d.Id() != "wan masquerade" {
		t.Fatalf("create set id %q, want the comment", d.Id())
	}
	if len(router.items) != 1 {
		t.Fatalf("router has %d items, want 1", len(router.items))
	}

	dup := natData(t, res, map[string]string{"chain": "srcnat", "action": "masquerade", "comment": "wan masquerade"})
	if dg := res.CreateContext(context.Background(), dup, client); !dg.HasError() {
		t.Fatal("duplicate-comment create succeeded, want uniqueness error")
	}
	if len(router.items) != 1 {
		t.Fatalf("duplicate create reached the router: %d items", len(router.items))
	}
}

func TestCommentIdentityReadSurvivesIDChurn(t *testing.T) {
	router, res, client := natHarness(t)
	// The item was deleted and recreated out-of-band: same comment, new .id.
	router.items["*7"] = map[string]string{".id": "*7", "chain": "srcnat", "action": "masquerade", "comment": "wan masquerade"}

	d := natData(t, res, map[string]string{})
	d.SetId("wan masquerade")
	if dg := res.ReadContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("read failed: %v", dg)
	}
	if d.Id() != "wan masquerade" {
		t.Fatalf("read left id %q, want the comment", d.Id())
	}
	if d.Get("chain").(string) != "srcnat" {
		t.Fatalf("read did not populate fields: chain=%q", d.Get("chain"))
	}
}

func TestCommentIdentityReadGoneAndAmbiguous(t *testing.T) {
	router, res, client := natHarness(t)

	d := natData(t, res, map[string]string{})
	d.SetId("missing")
	if dg := res.ReadContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("read of missing item errored: %v", dg)
	}
	if d.Id() != "" {
		t.Fatalf("read of missing item kept id %q, want cleared", d.Id())
	}

	router.items["*1"] = map[string]string{".id": "*1", "chain": "srcnat", "comment": "dup"}
	router.items["*2"] = map[string]string{".id": "*2", "chain": "srcnat", "comment": "dup"}
	d = natData(t, res, map[string]string{})
	d.SetId("dup")
	if dg := res.ReadContext(context.Background(), d, client); !dg.HasError() {
		t.Fatal("read with duplicate comments succeeded, want ambiguity error")
	}
}

func TestCommentIdentityUpdateRename(t *testing.T) {
	router, res, client := natHarness(t)
	router.items["*3"] = map[string]string{".id": "*3", "chain": "srcnat", "action": "masquerade", "comment": "old name"}

	d := natData(t, res, map[string]string{"chain": "srcnat", "action": "masquerade", "comment": "new name"})
	d.SetId("old name")
	if dg := res.UpdateContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("update failed: %v", dg)
	}
	if d.Id() != "new name" {
		t.Fatalf("update left id %q, want the new comment", d.Id())
	}
	if router.items["*3"]["comment"] != "new name" {
		t.Fatalf("router item comment is %q, want renamed", router.items["*3"]["comment"])
	}
}

func TestCommentIdentityDeleteTolerantOfGone(t *testing.T) {
	router, res, client := natHarness(t)
	router.items["*4"] = map[string]string{".id": "*4", "chain": "srcnat", "comment": "victim"}

	d := natData(t, res, map[string]string{})
	d.SetId("victim")
	if dg := res.DeleteContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("delete failed: %v", dg)
	}
	if len(router.items) != 0 {
		t.Fatal("delete did not remove the item")
	}

	d = natData(t, res, map[string]string{})
	d.SetId("victim")
	if dg := res.DeleteContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("delete of already-gone item errored: %v", dg)
	}
}

func TestCommentIdentityResourceGates(t *testing.T) {
	for _, name := range commentIdentityResources {
		if slices.Contains(nameIdentityResources, name) {
			t.Errorf("%s is in both comment and name identity lists", name)
		}
		upstream := routeros.Provider().ResourcesMap[name]
		if upstream == nil {
			t.Fatalf("%s is not an upstream resource", name)
		}
		if upstream.Schema["comment"] == nil {
			t.Errorf("%s has no comment field to use as identity", name)
		}
		if upstream.Schema["name"] != nil {
			t.Errorf("%s has a name field upstream; use name identity instead", name)
		}
		if got, _ := upstream.Schema[routeros.MetaId].Default.(int); got != int(routeros.Id) {
			t.Errorf("%s upstream ___id___ default = %v, want Id", name, got)
		}
	}
}
