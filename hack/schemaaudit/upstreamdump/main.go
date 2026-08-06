// upstreamdump emits the upstream terraform-provider-routeros resource
// schemas as JSON for the schema audit: resource name, RouterOS path,
// and per-field type/required/computed.
package main

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

type field struct {
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	Computed bool   `json:"computed,omitempty"`
}

type resource struct {
	Resource string           `json:"resource"`
	Path     string           `json:"path"`
	Fields   map[string]field `json:"fields"`
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
			if strings.HasPrefix(fname, "___") {
				continue
			}
			res.Fields[fname] = field{
				Type:     s.Type.String(),
				Required: s.Required,
				Optional: s.Optional,
				Computed: s.Computed,
			}
		}
		out = append(out, res)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Resource < out[j].Resource })
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}
