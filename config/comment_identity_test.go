package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

const (
	attrAction       = "action"
	attrChain        = "chain"
	attrID           = ".id"
	chainSrcnat      = "srcnat"
	actionMasquerade = "masquerade"
	actionAccept     = "accept"
	testComment      = "wan masquerade"
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
				if c, filtered := r.URL.Query()[commentField]; filtered && it[commentField] != c[0] {
					continue
				}
				if c, filtered := r.URL.Query()[attrID]; filtered && it[attrID] != c[0] {
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
			body[attrID] = fmt.Sprintf("*%X", f.nextID)
			f.items[body[attrID]] = body
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
	res := providerForRuntime().ResourcesMap["routeros_ip_firewall_nat"]
	return router, res, testClient(t, srv.URL)
}

// testClient constructs a client through NewClient like the real provider:
// RestClient has unexported fields (ctx, extra) that SendRequest
// dereferences.
func testClient(t *testing.T, hosturl string) routeros.Client {
	t.Helper()
	pd := schema.TestResourceDataRaw(t, routeros.Provider().Schema, map[string]any{
		"hosturl":          hosturl,
		"username":         "admin",
		"routeros_version": "7.23.2",
	})
	c, dg := routeros.NewClient(context.Background(), pd)
	if dg.HasError() {
		t.Fatalf("NewClient: %v", dg)
	}
	return c.(routeros.Client)
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
		} else if def, ok := s.Default.(string); ok && def != "" && s.Type == schema.TypeString {
			// Terraform materializes schema defaults into state before the
			// serializer runs; mirror that, or defaulted fields are sent as
			// "" (the router rejects e.g. address-list-timeout="").
			attrs[name] = def
			ctyVals[name] = cty.StringVal(def)
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
	case schema.TypeString, schema.TypeInvalid:
		return cty.String
	default:
		return cty.String
	}
}

func TestCommentIdentityCreate(t *testing.T) {
	router, res, client := natHarness(t)

	d := natData(t, res, map[string]string{attrChain: chainSrcnat, attrAction: actionMasquerade})
	if dg := res.CreateContext(context.Background(), d, client); !dg.HasError() {
		t.Fatal("create without comment succeeded, want identity error")
	}

	d = natData(t, res, map[string]string{attrChain: chainSrcnat, attrAction: actionMasquerade, commentField: testComment})
	if dg := res.CreateContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("create failed: %v", dg)
	}
	if d.Id() != testComment {
		t.Fatalf("create set id %q, want the comment", d.Id())
	}
	if len(router.items) != 1 {
		t.Fatalf("router has %d items, want 1", len(router.items))
	}

	dup := natData(t, res, map[string]string{attrChain: chainSrcnat, attrAction: actionMasquerade, commentField: testComment})
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
	router.items["*7"] = map[string]string{attrID: "*7", attrChain: chainSrcnat, attrAction: actionMasquerade, commentField: testComment}

	d := natData(t, res, map[string]string{})
	d.SetId(testComment)
	if dg := res.ReadContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("read failed: %v", dg)
	}
	if d.Id() != testComment {
		t.Fatalf("read left id %q, want the comment", d.Id())
	}
	if d.Get(attrChain).(string) != chainSrcnat {
		t.Fatalf("read did not populate fields: chain=%q", d.Get(attrChain))
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

	router.items["*1"] = map[string]string{attrID: "*1", attrChain: chainSrcnat, commentField: "dup"}
	router.items["*2"] = map[string]string{attrID: "*2", attrChain: chainSrcnat, commentField: "dup"}
	d = natData(t, res, map[string]string{})
	d.SetId("dup")
	if dg := res.ReadContext(context.Background(), d, client); !dg.HasError() {
		t.Fatal("read with duplicate comments succeeded, want ambiguity error")
	}
}

func TestCommentIdentityUpdateRename(t *testing.T) {
	router, res, client := natHarness(t)
	router.items["*3"] = map[string]string{attrID: "*3", attrChain: chainSrcnat, attrAction: actionMasquerade, commentField: "old name"}

	d := natData(t, res, map[string]string{attrChain: chainSrcnat, attrAction: actionMasquerade, commentField: "new name"})
	d.SetId("old name")
	if dg := res.UpdateContext(context.Background(), d, client); dg.HasError() {
		t.Fatalf("update failed: %v", dg)
	}
	if d.Id() != "new name" {
		t.Fatalf("update left id %q, want the new comment", d.Id())
	}
	if router.items["*3"][commentField] != "new name" {
		t.Fatalf("router item comment is %q, want renamed", router.items["*3"][commentField])
	}
}

func TestCommentIdentityDeleteTolerantOfGone(t *testing.T) {
	router, res, client := natHarness(t)
	router.items["*4"] = map[string]string{attrID: "*4", attrChain: chainSrcnat, commentField: "victim"}

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

// nameVerdicts loads the pinned per-resource name-uniqueness verdicts.
func nameVerdicts(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile("name-uniqueness.json")
	if err != nil {
		t.Fatalf("cannot read pinned verdicts: %v", err)
	}
	var pinned struct {
		Verdicts []struct {
			Resource string `json:"resource"`
			Verdict  string `json:"verdict"`
		} `json:"verdicts"`
	}
	if err := json.Unmarshal(data, &pinned); err != nil {
		t.Fatalf("cannot parse pinned verdicts: %v", err)
	}
	out := map[string]string{}
	for _, v := range pinned.Verdicts {
		out[v.Resource] = v.Verdict
	}
	return out
}

func TestFactoryIdentityResourceGates(t *testing.T) {
	for _, name := range factoryIdentityResources {
		if slices.Contains(nameIdentityResources, name) || slices.Contains(commentIdentityResources, name) {
			t.Errorf("%s is in multiple identity lists", name)
		}
		upstream := routeros.Provider().ResourcesMap[name]
		if upstream == nil {
			t.Fatalf("%s is not an upstream resource", name)
		}
		s := upstream.Schema[factoryNameField]
		if s == nil || !s.Required {
			t.Errorf("%s must have a required %s field to serve as identity", name, factoryNameField)
		}
		if got, _ := upstream.Schema[routeros.MetaId].Default.(int); got != int(routeros.Id) {
			t.Errorf("%s upstream ___id___ default = %v, want Id", name, got)
		}
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
		if upstream.Schema[commentField] == nil && !slices.Contains(injectedCommentResources, name) {
			t.Errorf("%s has no comment field to use as identity", name)
		}
		if upstream.Schema["name"] != nil && nameVerdicts(t)[name] == "UNIQUE" {
			t.Errorf("%s has an enforced-unique name (pinned verdict); use name identity instead", name)
		}
		if got, _ := upstream.Schema[routeros.MetaId].Default.(int); got != int(routeros.Id) {
			t.Errorf("%s upstream ___id___ default = %v, want Id", name, got)
		}
	}
	for _, name := range injectedCommentResources {
		if !slices.Contains(commentIdentityResources, name) {
			t.Errorf("%s has an injected comment but is not in commentIdentityResources; the injection is pointless without comment identity", name)
		}
		if routeros.Provider().ResourcesMap[name].Schema[commentField] != nil {
			t.Errorf("%s now models comment upstream; drop it from injectedCommentResources", name)
		}
		for which, p := range map[string]*schema.Provider{"runtime": providerForRuntime(), "generation": providerForGeneration()} {
			if p.ResourcesMap[name].Schema[commentField] == nil {
				t.Errorf("%s %s schema is missing the injected comment field", which, name)
			}
		}
	}
}
