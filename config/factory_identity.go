package config

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// factoryIdentityResources are physical-hardware resources identified by the
// router's immutable default-name (the Terraform factory_name field, e.g.
// ether8). The default-name survives interface renames, configuration resets,
// and reinstalls, while the internal .id is reassigned on rebuild — with .id
// identity a post-rebuild reconcile could resolve to the wrong physical port.
// The name field is deliberately not the identity: it is mutable and managed
// by this very resource. Upstream create already adopts the port by
// factory_name but stores the .id; this wrapper keeps the factory name as the
// external identity and resolves the current .id on every operation. Delete
// is not wrapped: physical ports cannot be deleted, and upstream's delete
// only clears state without touching the router.
var factoryIdentityResources = []string{
	"routeros_interface_ethernet",
}

const (
	factoryNameField = "factory_name"
	defaultNameREST  = "default-name"
)

func withFactoryIdentity(p *schema.Provider) *schema.Provider {
	for _, name := range factoryIdentityResources {
		r := p.ResourcesMap[name]
		path, _ := r.Schema[routeros.MetaResourcePath].Default.(string)
		r.CreateContext = factoryIdentityCreate(r.CreateContext)
		r.ReadContext = factoryIdentityResolve(path, r.ReadContext, true)
		r.UpdateContext = factoryIdentityResolve(path, r.UpdateContext, false)
	}
	return p
}

// factoryIdentityCreate delegates to the upstream adopt-by-factory_name
// create, then replaces the stored .id with the factory name. Existence is
// enforced by the upstream lookup itself.
func factoryIdentityCreate(create schema.CreateContextFunc) schema.CreateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		dg := create(ctx, d, m)
		if dg.HasError() {
			return dg
		}
		d.SetId(d.Get(factoryNameField).(string))
		return dg
	}
}

// factoryIdentityResolve maps the factory name in d.Id() to the current .id,
// delegates, and restores the factory name. A vanished port clears state on
// read (goneOK) and errors on update.
func factoryIdentityResolve[T ~func(context.Context, *schema.ResourceData, any) diag.Diagnostics](path string, op T, goneOK bool) T {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		factory := d.Id()
		item, dg := resolveByField(m, path, defaultNameREST, factory)
		if dg != nil {
			return dg
		}
		if item == nil {
			if goneOK {
				d.SetId("")
				return nil
			}
			return diag.Errorf("no interface with %s %q at %s", defaultNameREST, factory, path)
		}
		d.SetId(item.GetID(routeros.Id))
		dg = op(ctx, d, m)
		if dg.HasError() {
			return dg
		}
		if d.Id() != "" {
			d.SetId(factory)
		}
		return dg
	}
}
