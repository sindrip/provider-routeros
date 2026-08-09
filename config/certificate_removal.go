package config

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// certificateRemovalResources delete through an upstream path that revokes
// without removing. For a certificate issued by a CA, upstream posts to
// /certificate/issued-revoke and stops: the row survives with revoked=true
// (verified against 7.23.2) while the resource's state is cleared, so the
// managed resource is gone and the certificate is not. Because the name is
// enforced unique, that stranded row also makes the resource unrecreatable —
// the next create is answered "Name exists, please choose another!" — which is
// exactly the reconverge a reset is supposed to allow.
//
// Revoking first is correct and kept: it puts the serial beyond use, which
// deleting the row alone would not. It is simply not a delete. This wrapper
// follows upstream with the removal the router does support, a plain DELETE of
// the row by .id. A root CA takes upstream's remove branch and is already gone
// by then, so the follow-up finds nothing and does nothing.
var certificateRemovalResources = []string{
	"routeros_system_certificate",
}

func withCertificateRemoval(p *schema.Provider) *schema.Provider {
	for _, name := range certificateRemovalResources {
		r := p.ResourcesMap[name]
		path, _ := r.Schema[routeros.MetaResourcePath].Default.(string)
		r.DeleteContext = certificateRemoval(path, r.DeleteContext)
	}
	return p
}

func certificateRemoval(path string, del schema.DeleteContextFunc) schema.DeleteContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		// Read the name before delegating: upstream clears the id on success,
		// and under name identity the id is the only place the name is held.
		name := d.Get(routeros.KeyName).(string)
		dg := del(ctx, d, m)
		if dg.HasError() {
			return dg
		}
		if name == "" {
			return dg
		}
		item, resolveDg := resolveByField(m, path, routeros.KeyName, name)
		if resolveDg != nil {
			return append(dg, resolveDg...)
		}
		if item == nil {
			// Upstream removed the row outright; nothing was revoked.
			return dg
		}
		if err := routeros.DeleteItem(&routeros.ItemId{Type: routeros.Id, Value: item.GetID(routeros.Id)},
			path, m.(routeros.Client)); err != nil {
			return append(dg, diag.FromErr(err)...)
		}
		return dg
	}
}
