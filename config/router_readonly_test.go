package config

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// Moving a field out of spec is a CRD change, so it has to reach the
// generation schema as well as the runtime one.
func TestRouterReadOnlyBothSchemas(t *testing.T) {
	for which, p := range map[string]*schema.Provider{
		"runtime":    providerForRuntime(),
		"generation": providerForGeneration(),
	} {
		for resource, fields := range routerReadOnlyFields {
			for _, field := range fields {
				s := p.ResourcesMap[resource].Schema[field]
				if s == nil {
					t.Fatalf("%s: %s has no %s field", which, resource, field)
				}
				if !s.Computed {
					t.Errorf("%s: %s.%s is not Computed; it will not be observed", which, resource, field)
				}
				if s.Optional || s.Required {
					t.Errorf("%s: %s.%s is still settable (optional=%v required=%v); "+
						"it stays in spec, where the router refuses it",
						which, resource, field, s.Optional, s.Required)
				}
				if s.Default != nil {
					t.Errorf("%s: %s.%s keeps default %v", which, resource, field, s.Default)
				}
			}
		}
	}
}

// Upstream must still be offering the field as settable, or the override is
// redundant and should be dropped rather than left to rot.
func TestRouterReadOnlyGates(t *testing.T) {
	for resource, fields := range routerReadOnlyFields {
		upstream := routeros.Provider().ResourcesMap[resource]
		if upstream == nil {
			t.Fatalf("%s is not an upstream resource", resource)
		}
		for _, field := range fields {
			s := upstream.Schema[field]
			if s == nil {
				t.Errorf("%s.%s no longer exists upstream; drop it from routerReadOnlyFields", resource, field)
				continue
			}
			if !s.Optional && !s.Required {
				t.Errorf("%s.%s is already computed-only upstream; the override is redundant, drop it",
					resource, field)
			}
		}
	}
}
