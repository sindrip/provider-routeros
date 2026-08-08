package config

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// policyExpansionResources have a RouterOS "policy" set that the router stores
// as a full partition: every permission keyword the group does not grant is
// written back with a "!" prefix. A spec that lists only the granted
// permissions -- e.g. read,api,rest-api -- therefore reads back as those three
// plus an explicit negation of every other keyword (17 members on CHR 7.23.2).
//
// The observed set (17) never equals the desired set (3), so a managed
// reconciler issues an Update on every pass: a set that never converges,
// re-expanded by the router and diffed again forever. The upstream field
// carries a DiffSuppressFunc built for exactly this "!x vs absent" case, and
// it converges in plain Terraform -- but the managed-reconcile update decision
// compares desired against observed above the SDK diff, where that suppression
// never runs, so the write loop stands. On flash-backed hardware it is real
// eMMC wear, not just log spam.
//
// Collapsing the observed policy to its granted (non-negated) members on read
// removes the difference at its source: observed equals desired at every
// layer, so no diff mechanism has to be trusted to hide it. RouterOS user
// groups have no inheritance, so a "!keyword" is exactly "keyword omitted";
// dropping negations is lossless. The contract this assumes: specs express
// policy as the set of granted permissions (positive members). A spec that
// declares an explicit negation is redundant here and is not round-tripped.
var policyExpansionResources = []string{
	"routeros_system_user_group",
}

const policyField = "policy"

func withoutPolicyExpansion(p *schema.Provider) *schema.Provider {
	for _, name := range policyExpansionResources {
		r := p.ResourcesMap[name]
		r.ReadContext = collapsePolicyRead(r.ReadContext)
	}
	return p
}

// collapsePolicyRead runs the wrapped read, then drops the "!"-prefixed members
// RouterOS adds when it stores the policy set as a full partition, leaving the
// granted permissions the spec declares.
func collapsePolicyRead(read schema.ReadContextFunc) schema.ReadContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		dg := read(ctx, d, m)
		if dg.HasError() || d.Id() == "" {
			return dg
		}
		raw, ok := d.GetOk(policyField)
		if !ok {
			return dg
		}
		set, ok := raw.(*schema.Set)
		if !ok {
			return dg
		}
		granted := make([]any, 0, set.Len())
		for _, v := range set.List() {
			if s, _ := v.(string); !strings.HasPrefix(s, "!") {
				granted = append(granted, v)
			}
		}
		if len(granted) == set.Len() {
			return dg
		}
		if err := d.Set(policyField, granted); err != nil {
			return diag.FromErr(err)
		}
		return dg
	}
}
