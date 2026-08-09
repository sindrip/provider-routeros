package config

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

const (
	advertiseDNSField = "advertise_dns"
	advertiseDNSREST  = "advertise-dns"
	advertiseDNSSelf  = "self"
)

// The retype has to reach the generation schema as well as the runtime one:
// the CRD's string is what has to deserialize into the runtime field.
func TestMistypedEnumsRetypesBothSchemas(t *testing.T) {
	for which, p := range map[string]*schema.Provider{
		schemaRuntime:    providerForRuntime(),
		schemaGeneration: providerForGeneration(),
	} {
		for resource, fields := range mistypedFields {
			for _, f := range fields {
				s := p.ResourcesMap[resource].Schema[f.field]
				if s == nil {
					t.Fatalf("%s: %s has no %s field", which, resource, f.field)
				}
				if s.Type != schema.TypeString {
					t.Errorf("%s: %s.%s type = %v, want string", which, resource, f.field, s.Type)
				}
				if s.Default != nil {
					t.Errorf("%s: %s.%s keeps default %v; a defaulted field is sent on every request",
						which, resource, f.field, s.Default)
				}
				if f.doc != "" && !strings.Contains(s.Description, f.doc) {
					t.Errorf("%s: %s.%s description does not carry the accepted values", which, resource, f.field)
				}
			}
		}
	}
}

// Upstream must still be modelling it as a bool, or the override is redundant
// and should be dropped rather than left to rot.
func TestMistypedEnumsGates(t *testing.T) {
	for resource, fields := range mistypedFields {
		upstream := routeros.Provider().ResourcesMap[resource]
		if upstream == nil {
			t.Fatalf("%s is not an upstream resource", resource)
		}
		for _, f := range fields {
			s := upstream.Schema[f.field]
			if s == nil {
				t.Errorf("%s.%s no longer exists upstream; drop it from mistypedFields", resource, f.field)
				continue
			}
			if s.Type != f.upstream {
				t.Errorf("%s.%s is now %v upstream, not %v; the retype is redundant or the entry is stale, re-triage it",
					resource, f.field, s.Type, f.upstream)
			}
		}
	}
}
