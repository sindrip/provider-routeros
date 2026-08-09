package config

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// The mark has to reach the generation schema as well as the runtime one:
// Sensitive is what upjet keys off to emit a SecretKeySelector and to keep the
// value out of the observation.
func TestSensitiveFieldsBothSchemas(t *testing.T) {
	for which, p := range map[string]*schema.Provider{
		schemaRuntime:    providerForRuntime(),
		schemaGeneration: providerForGeneration(),
	} {
		for resource, fields := range sensitiveFields {
			for _, field := range fields {
				s := p.ResourcesMap[resource].Schema[field]
				if s == nil {
					t.Fatalf("%s: %s has no %s field", which, resource, field)
				}
				if !s.Sensitive {
					t.Errorf("%s: %s.%s is not Sensitive; it generates as a plain string "+
						"and the observed secret lands in status.atProvider",
						which, resource, field)
				}
			}
		}
	}
}

// Upstream must still be leaving them unmarked, or the override is redundant
// and should be dropped rather than left to rot.
func TestSensitiveFieldsGates(t *testing.T) {
	for resource, fields := range sensitiveFields {
		upstream := routeros.Provider().ResourcesMap[resource]
		if upstream == nil {
			t.Fatalf("%s is not an upstream resource", resource)
		}
		for _, field := range fields {
			s := upstream.Schema[field]
			if s == nil {
				t.Errorf("%s.%s no longer exists upstream; drop it from sensitiveFields", resource, field)
				continue
			}
			if s.Sensitive {
				t.Errorf("%s.%s is now Sensitive upstream; the override is redundant, drop it",
					resource, field)
			}
		}
	}
}

// A secret upstream already marks Sensitive must not be in the list, and is
// the reference for what the override produces. preshared_key sits on the same
// resource as the peer's private_key and was always modelled correctly, which
// is what made the omission a defect rather than a decision.
func TestSensitiveFieldsLeaveCorrectlyMarkedAlone(t *testing.T) {
	for _, ref := range []struct{ resource, field string }{
		{"routeros_interface_wireguard_peer", "preshared_key"},
		{"routeros_interface_wireguard", fieldPrivateKey},
	} {
		s := routeros.Provider().ResourcesMap[ref.resource].Schema[ref.field]
		if s == nil {
			t.Fatalf("%s.%s no longer exists upstream", ref.resource, ref.field)
		}
		if !s.Sensitive {
			t.Errorf("%s.%s is no longer Sensitive upstream; it now needs the override too",
				ref.resource, ref.field)
		}
		for _, field := range sensitiveFields[ref.resource] {
			if field == ref.field {
				t.Errorf("%s.%s is already Sensitive upstream; remove it from sensitiveFields",
					ref.resource, ref.field)
			}
		}
	}
}
