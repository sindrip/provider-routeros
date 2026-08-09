package config

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// maskedDiffFields are Sensitive fields upstream also marks Computed, which
// under upjet is a combination that cannot converge.
//
// Upjet stores a sensitive attribute in its state redacted, as the literal
// "*****". Computed is what makes it rebuild that attribute's prior state from
// the observation rather than from the configuration, so the plan ends up
// comparing the redacted state against the real value resolved from the
// Secret. Those are never equal. The diff never closes, an update fires, the
// request omits sensitive fields — leaving a payload of nothing but the
// writable non-sensitive ones — the device is unchanged, and it repeats at
// reconcile speed. Observed at ~10 writes/second against a live router, with
// the managed resource reporting Synced and Ready throughout, which is what
// makes it expensive: nothing surfaces until the router's flash starts wearing.
//
// Clearing Computed leaves the field Optional and Sensitive, so its prior
// state comes from the configuration and equals what the Secret resolves to.
// A spec that declares the key converges; a spec that omits it puts nothing in
// the configuration at all, so there is still nothing to disagree with. What
// is given up is the field being reported in the observation — which for a
// private key is not a loss, and v0.25.0 removed fifteen other secrets from
// there deliberately. The value still reaches the connection secret, which is
// where an observed secret belongs.
//
// Only fields that are Sensitive *and* Computed belong here. The fifteen
// v0.25.0 marked Sensitive are Optional alone: their prior state already comes
// from the configuration, so they never had this diff and clearing anything on
// them would only put the secrets back in the observation.
//
// The defect is upjet's, not the RouterOS provider's, and the honest fix is to
// stop diffing against a redacted value — keep sensitive attributes intact in
// the state used for planning and redact on the way out, or compare a digest.
// This is the narrowest change that stops a live router being written to
// forever, not an argument that the layer is right.
var maskedDiffFields = map[string][]string{
	"routeros_interface_wireguard": {"private_key"},
}

func withoutMaskedDiff(p *schema.Provider) *schema.Provider {
	for resource, fields := range maskedDiffFields {
		r := p.ResourcesMap[resource]
		if r == nil {
			panic("maskedDiffFields: no upstream resource " + resource)
		}
		for _, field := range fields {
			s := r.Schema[field]
			if s == nil {
				panic("maskedDiffFields: " + resource + " has no field " + field)
			}
			if !s.Sensitive {
				panic("maskedDiffFields: " + resource + "." + field +
					" is not Sensitive; it never had the redacted-state diff")
			}
			s.Computed = false
		}
	}
	return p
}
