package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// consoleArgs loads the pinned console tree and returns, per menu path, the
// set of writable argument names (add/set command arguments).
func consoleArgs(t *testing.T) map[string]map[string]bool {
	t.Helper()
	data, err := os.ReadFile("console-tree.json")
	if err != nil {
		t.Fatalf("cannot read pinned console tree: %v", err)
	}
	type node struct {
		Name     string `json:"name"`
		NodeType string `json:"node_type"`
		Children []node `json:"children"`
	}
	var tree struct {
		Tree []node `json:"tree"`
	}
	if err := json.Unmarshal(data, &tree); err != nil {
		t.Fatalf("cannot parse pinned console tree: %v", err)
	}
	out := map[string]map[string]bool{}
	var walk func(nodes []node, path string)
	walk = func(nodes []node, path string) {
		for _, n := range nodes {
			if n.NodeType != "dir" && n.NodeType != "path" {
				continue
			}
			p := path + "/" + n.Name
			args := map[string]bool{}
			for _, cmd := range n.Children {
				if cmd.NodeType != "cmd" || (cmd.Name != "add" && cmd.Name != "set") {
					continue
				}
				for _, a := range cmd.Children {
					if a.NodeType == "arg" {
						args[a.Name] = true
					}
				}
			}
			out[p] = args
			walk(n.Children, p)
		}
	}
	walk(tree.Tree, "")
	return out
}

// TestPhantomDefaultFieldGates pins each phantom entry against the reasons
// it is in the list: the upstream field exists and carries a default (so
// the serializer would always send it), it is not already skip-listed
// upstream, and the router's console tree has no such argument. If upstream
// fixes a field or the router learns the argument, this fails and the entry
// must be removed.
func TestPhantomDefaultFieldGates(t *testing.T) {
	console := consoleArgs(t)
	runtime := providerForRuntime()
	for name, fields := range phantomDefaultFields {
		upstream := routeros.Provider().ResourcesMap[name]
		if upstream == nil {
			t.Fatalf("%s is not an upstream resource", name)
		}
		path, _ := upstream.Schema[routeros.MetaResourcePath].Default.(string)
		args, ok := console[path]
		if !ok {
			t.Fatalf("%s: no console-tree menu at %s", name, path)
		}
		var skip string
		if sf := upstream.Schema[routeros.MetaSkipFields]; sf != nil {
			skip, _ = sf.Default.(string)
		}
		for _, field := range fields {
			s := upstream.Schema[field]
			if s == nil {
				t.Errorf("%s.%s is not in the upstream schema; drop it from the phantom list", name, field)
				continue
			}
			if s.Default == nil {
				t.Errorf("%s.%s has no upstream default (fixed upstream?); drop it from the phantom list", name, field)
			}
			if strings.Contains(skip, `"`+field+`"`) {
				t.Errorf("%s.%s is skip-listed upstream and never serialized; drop it from the phantom list", name, field)
			}
			if arg := strings.ReplaceAll(field, "_", "-"); args[arg] {
				t.Errorf("%s.%s: the router accepts %q at %s; not a phantom", name, field, arg, path)
			}
			if runtime.ResourcesMap[name].Schema[field].Default != nil {
				t.Errorf("%s.%s still has a default in the runtime schema", name, field)
			}
		}
	}
}

// TestPhantomDefaultsNotSerialized proves the fix at the serializer: with
// the runtime schema an unset phantom field stays out of the request, while
// the unmodified upstream schema sends it on every create — the bug that
// made BGP connections impossible to create.
func TestPhantomDefaultsNotSerialized(t *testing.T) {
	vals := map[string]string{"name": "core", "as": "65000"}

	fixed := providerForRuntime().ResourcesMap["routeros_routing_bgp_connection"]
	item, _ := routeros.TerraformResourceDataToMikrotik(fixed.Schema, natData(t, fixed, vals))
	if v, ok := item["add-path-out"]; ok {
		t.Errorf("runtime schema still serializes add-path-out=%q for an unset field", v)
	}

	upstream := routeros.Provider().ResourcesMap["routeros_routing_bgp_connection"]
	item, _ = routeros.TerraformResourceDataToMikrotik(upstream.Schema, natData(t, upstream, vals))
	if _, ok := item["add-path-out"]; !ok {
		t.Error("upstream schema no longer serializes add-path-out for an unset field (fixed upstream?); revisit the phantom list")
	}
	// address_families is deliberately NOT phantom: upstream's drift table
	// rewrites it to afi for the pinned router version. If afi stops being
	// sent, the drift compensation broke and the field must be re-triaged.
	if _, ok := item["afi"]; !ok {
		t.Error("upstream drift compensation no longer rewrites address_families to afi; re-triage the field")
	}
	if _, ok := item["address-families"]; ok {
		t.Error("upstream serializes raw address-families despite the drift table; add it to the phantom list")
	}
}
