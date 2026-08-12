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
		schemaRuntime:    providerForRuntime(),
		schemaGeneration: providerForGeneration(),
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

// Upstream shares one *schema.Schema across resources — PropVrfRw is a single
// pointer behind every menu that takes vrf as a set argument — so a demotion
// written in place travels to menus the router does let write the field. Only
// the resource we name may come out changed.
//
// The baseline is read as values, not as a second provider to compare against
// later: an in-place demotion would follow the shared pointer into any provider
// still holding it, baseline included, and the comparison would agree with
// itself while both were wrong.
func TestRouterReadOnlyDemotesOnlyTheNamedResource(t *testing.T) {
	type settability struct{ optional, required, computed bool }
	baseline := map[string]map[string]settability{}
	for name, r := range routeros.Provider().ResourcesMap {
		fields := map[string]settability{}
		for field, s := range r.Schema {
			fields[field] = settability{s.Optional, s.Required, s.Computed}
		}
		baseline[name] = fields
	}

	after := withRouterReadOnly(routeros.Provider())
	for demoted, fields := range routerReadOnlyFields {
		for _, field := range fields {
			for name, r := range after.ResourcesMap {
				if name == demoted || r.Schema[field] == nil {
					continue
				}
				s := r.Schema[field]
				was := baseline[name][field]
				if s.Optional != was.optional || s.Required != was.required || s.Computed != was.computed {
					t.Errorf("demoting %s.%s reached %s.%s: optional %v->%v required %v->%v computed %v->%v",
						demoted, field, name, field,
						was.optional, s.Optional, was.required, s.Required, was.computed, s.Computed)
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
