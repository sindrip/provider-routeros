package config

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// Computed is what makes upjet rebuild the attribute's prior state from the
// redacted observation, so clearing it has to reach the runtime schema. It is
// applied to the generation schema too, though generation is unchanged by it:
// the field is Sensitive, so it was already a SecretRef in the spec and absent
// from the observation.
func TestMaskedDiffClearsComputed(t *testing.T) {
	for which, p := range map[string]*schema.Provider{
		schemaRuntime:    providerForRuntime(),
		schemaGeneration: providerForGeneration(),
	} {
		for resource, fields := range maskedDiffFields {
			for _, field := range fields {
				s := p.ResourcesMap[resource].Schema[field]
				if s == nil {
					t.Fatalf("%s: %s has no %s field", which, resource, field)
				}
				if s.Computed {
					t.Errorf("%s: %s.%s is still Computed; its prior state comes from the "+
						"redacted observation and the diff cannot close", which, resource, field)
				}
				if !s.Sensitive {
					t.Errorf("%s: %s.%s lost Sensitive; the secret returns to the observation",
						which, resource, field)
				}
			}
		}
	}
}

// Upstream must still ship the combination, or the override is redundant. It
// is upstream's marking, not this provider's: the loop predates every
// sensitivity change here, which v0.24.0 reproducing it confirms.
func TestMaskedDiffGates(t *testing.T) {
	for resource, fields := range maskedDiffFields {
		upstream := routeros.Provider().ResourcesMap[resource]
		if upstream == nil {
			t.Fatalf("%s is not an upstream resource", resource)
		}
		for _, field := range fields {
			s := upstream.Schema[field]
			if s == nil {
				t.Errorf("%s.%s no longer exists upstream; drop it from maskedDiffFields", resource, field)
				continue
			}
			if !s.Sensitive || !s.Computed {
				t.Errorf("%s.%s is no longer Sensitive+Computed upstream (sensitive=%v computed=%v); "+
					"the override is redundant, drop it", resource, field, s.Sensitive, s.Computed)
			}
		}
	}
}

// The fields v0.25.0 marked Sensitive are Optional alone. Their prior state
// comes from the configuration, so they never had this diff — and clearing
// anything on them would only put the secrets back in the observation. If one
// ever gains Computed upstream, it needs adding to maskedDiffFields.
func TestSensitiveFieldsAreNotComputed(t *testing.T) {
	for resource, fields := range sensitiveFields {
		for _, field := range fields {
			s := routeros.Provider().ResourcesMap[resource].Schema[field]
			if s == nil {
				continue // the sensitiveFields gate reports this
			}
			if s.Computed {
				t.Errorf("%s.%s is now Sensitive+Computed upstream; it has the redacted-state "+
					"diff and belongs in maskedDiffFields", resource, field)
			}
		}
	}
}
