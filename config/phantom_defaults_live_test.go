package config

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// natDataBlocks is natData extended with single-entry nested blocks
// (TypeList with a resource element), which the serializer walks through
// GetRawConfig per nested field. Unlike natData it also materializes schema
// defaults into state, as the SDK's diff machinery does in production — the
// mechanism that makes defaulted fields part of every request.
func natDataBlocks(t *testing.T, res *schema.Resource, vals map[string]string, blocks map[string]map[string]string) *schema.ResourceData {
	t.Helper()
	attrs := map[string]string{}
	ctyVals := map[string]cty.Value{}
	for name, s := range res.Schema {
		if block, ok := blocks[name]; ok {
			elem := s.Elem.(*schema.Resource)
			attrs[name+".#"] = "1"
			objVals := map[string]cty.Value{}
			for elemName, elemSchema := range elem.Schema {
				if v, ok := block[elemName]; ok {
					attrs[name+".0."+elemName] = v
					objVals[elemName] = cty.StringVal(v)
				} else {
					objVals[elemName] = cty.NullVal(testCtyType(elemSchema))
				}
			}
			ctyVals[name] = cty.ListVal([]cty.Value{cty.ObjectVal(objVals)})
			continue
		}
		if v, ok := vals[name]; ok {
			attrs[name] = v
			ctyVals[name] = cty.StringVal(v)
			continue
		}
		ctyVals[name] = cty.NullVal(testCtyType(s))
		if s.Default == nil || strings.HasPrefix(name, "___") {
			continue
		}
		switch v := s.Default.(type) {
		case string:
			attrs[name] = v
		case bool:
			attrs[name] = strconv.FormatBool(v)
		case int:
			attrs[name] = strconv.Itoa(v)
		}
	}
	return res.Data(&terraform.InstanceState{Attributes: attrs, RawConfig: cty.ObjectVal(ctyVals)})
}

// TestPhantomDefaultsLiveCHRBgpConnection creates a BGP connection through
// the runtime provider — impossible before the phantom defaults were
// dropped, because every create carried add-path-out and address-families
// and the router rejects unknown parameters.
func TestPhantomDefaultsLiveCHRBgpConnection(t *testing.T) {
	host := os.Getenv("CHR_REST")
	if host == "" {
		t.Skip("set CHR_REST to run against a live CHR")
	}

	client := testClient(t, host)
	ctx := context.Background()

	// The router must still reject the phantom argument; if this create ever
	// succeeds, the router learned it and the pinned list is stale.
	if _, err := routeros.CreateItem(ctx, routeros.MikrotikItem{attrName: "ci-phantom", "add-path-out": "none"}, "/routing/bgp/connection", client); err == nil {
		t.Fatal("router accepted add-path-out; the phantom-defaults list is stale")
	}

	inst, err := routeros.CreateItem(ctx, routeros.MikrotikItem{attrName: "ci-live-bgp", "as": "65000"}, "/routing/bgp/instance", client)
	if err != nil {
		t.Fatalf("instance fixture: %v", err)
	}
	defer routeros.DeleteItem(&routeros.ItemId{Type: routeros.Id, Value: inst.GetID(routeros.Id)}, "/routing/bgp/instance", client) //nolint:errcheck

	res := providerForRuntime().ResourcesMap["routeros_routing_bgp_connection"]
	d := natDataBlocks(t, res,
		map[string]string{attrName: "ci-live-conn", "as": "65000", "instance": "ci-live-bgp", "listen": "true"},
		map[string]map[string]string{"local": {"role": "ibgp"}})
	if dg := res.CreateContext(ctx, d, client); dg.HasError() {
		t.Fatalf("create: %v", dg)
	}
	if d.Id() == "" {
		t.Fatal("create did not set an id")
	}
	defer routeros.DeleteItem(&routeros.ItemId{Type: routeros.Id, Value: d.Id()}, "/routing/bgp/connection", client) //nolint:errcheck

	rd := natDataBlocks(t, res, map[string]string{}, nil)
	rd.SetId(d.Id())
	if dg := res.ReadContext(ctx, rd, client); dg.HasError() {
		t.Fatalf("read: %v", dg)
	}
	if rd.Get(attrName).(string) != "ci-live-conn" {
		t.Fatalf("read did not populate name: %q", rd.Get(attrName))
	}
}
