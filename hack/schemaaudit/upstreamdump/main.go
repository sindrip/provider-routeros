// upstreamdump emits the upstream terraform-provider-routeros resource
// schemas as JSON for the schema audit: resource name, RouterOS path,
// serializer skip-list, and per-field type/required/computed/default.
// Fields nested one level inside block attributes are emitted as
// "outer.inner", matching the dotted argument names the serializer sends
// and the console tree lists.
package main

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	tfschema "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

type field struct {
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	Computed bool   `json:"computed,omitempty"`
	Default  any    `json:"default,omitempty"`
}

type resource struct {
	Resource   string           `json:"resource"`
	Path       string           `json:"path"`
	SkipFields []string         `json:"skip_fields,omitempty"`
	Fields     map[string]field `json:"fields"`
}

func main() {
	p := routeros.Provider()
	var out []resource
	for name, r := range p.ResourcesMap {
		res := resource{Resource: name, Fields: map[string]field{}}
		for fname, s := range r.Schema {
			if fname == routeros.MetaResourcePath {
				res.Path, _ = s.Default.(string)
			}
			if fname == routeros.MetaSkipFields {
				raw, _ := s.Default.(string)
				for _, f := range strings.Split(raw, ",") {
					if f = strings.Trim(strings.TrimSpace(f), `"`); f != "" {
						res.SkipFields = append(res.SkipFields, f)
					}
				}
				sort.Strings(res.SkipFields)
			}
			if strings.HasPrefix(fname, "___") {
				continue
			}
			res.Fields[fname] = field{
				Type:     s.Type.String(),
				Required: s.Required,
				Optional: s.Optional,
				Computed: s.Computed,
				Default:  s.Default,
			}
			if elem, ok := s.Elem.(*tfschema.Resource); ok {
				for nested, ns := range elem.Schema {
					res.Fields[fname+"."+nested] = field{
						Type:     ns.Type.String(),
						Required: ns.Required,
						Optional: ns.Optional,
						Computed: ns.Computed,
						Default:  ns.Default,
					}
				}
			}
		}
		out = append(out, res)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Resource < out[j].Resource })
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}
