package config

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// interfaceIdentityResources are resources whose menu holds at most one item
// per interface — the router enforces it, answering a second add with
// "configuration for this interface already exists" (verified against 7.23.2)
// — so the interface, not the reassignable .id, is the identity.
//
// /ipv6/nd also ships one router-owned row that can be neither added nor
// removed: it carries default=true, covers interface=all out of the box, and
// answers a delete with "can not remove default rule". Upstream create is a
// plain add, so that row was unmanageable — the add always collided with the
// row already occupying the interface, and the only way to keep the menu
// converged was to leave it out of Crossplane entirely. Create therefore
// adopts the default row by updating it in place, and delete releases it
// without touching the router. The rest of the menu keeps ordinary create and
// delete semantics.
//
// default=true is a property of the row, not of interface=all: the router
// accepts a rename of the default row onto another interface (verified), so
// adoption keys on the flag and not on the interface name.
//
// See docs/adr/0003 for why the interface is the identity here rather than the
// .id, a comment, or a hardcoded *N.
var interfaceIdentityResources = []string{
	"routeros_ipv6_neighbor_discovery",
}

const (
	interfaceField = "interface"
	defaultField   = "default"

	// routerTrue is how RouterOS spells a set boolean in a REST payload.
	routerTrue = "true"
)

func withInterfaceIdentity(p *schema.Provider) *schema.Provider {
	for _, name := range interfaceIdentityResources {
		r := p.ResourcesMap[name]
		path, _ := r.Schema[routeros.MetaResourcePath].Default.(string)
		// Capture the upstream callbacks before reassigning any of them:
		// create delegates to the unwrapped update when it adopts.
		create, read, update, del := r.CreateContext, r.ReadContext, r.UpdateContext, r.DeleteContext
		r.CreateContext = interfaceIdentityCreate(path, create, update)
		r.ReadContext = interfaceIdentityRead(path, read)
		r.UpdateContext = interfaceIdentityUpdate(path, update)
		r.DeleteContext = interfaceIdentityDelete(path, del)
	}
	return p
}

// interfaceIdentityCreate adds the item when the interface is free, adopts the
// router's default row when that row is what occupies the interface, and
// refuses to take over an item somebody else placed there.
func interfaceIdentityCreate(path string, create schema.CreateContextFunc, update schema.UpdateContextFunc) schema.CreateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		iface := d.Get(interfaceField).(string)
		if iface == "" {
			return diag.Errorf("interface is required: items at %s are one per interface, and the interface is the resource identity", path)
		}
		item, dg := resolveByField(m, path, interfaceField, iface)
		if dg != nil {
			return dg
		}
		switch {
		case item == nil:
			if dg := create(ctx, d, m); dg.HasError() {
				return dg
			}
		case (*item)[defaultField] == routerTrue:
			d.SetId(item.GetID(routeros.Id))
			if dg := update(ctx, d, m); dg.HasError() {
				return dg
			}
		default:
			return diag.Errorf("an item for interface %q already exists at %s (id %s): the interface is the resource identity and must be unique",
				iface, path, item.GetID(routeros.Id))
		}
		d.SetId(iface)
		return nil
	}
}

func interfaceIdentityRead(path string, read schema.ReadContextFunc) schema.ReadContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		item, dg := resolveByField(m, path, interfaceField, d.Id())
		if dg != nil {
			return dg
		}
		if item == nil {
			d.SetId("")
			return nil
		}
		d.SetId(item.GetID(routeros.Id))
		dg = read(ctx, d, m)
		if dg.HasError() {
			return dg
		}
		if d.Id() != "" {
			d.SetId(d.Get(interfaceField).(string))
		}
		return dg
	}
}

func interfaceIdentityUpdate(path string, update schema.UpdateContextFunc) schema.UpdateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		newInterface := d.Get(interfaceField).(string)
		if newInterface == "" {
			return diag.Errorf("interface cannot be cleared: it is the identity of items at %s", path)
		}
		item, dg := resolveByField(m, path, interfaceField, d.Id())
		if dg != nil {
			return dg
		}
		if item == nil {
			return diag.Errorf("item for interface %q no longer exists at %s", d.Id(), path)
		}
		if newInterface != d.Id() {
			existing, dg := resolveByField(m, path, interfaceField, newInterface)
			if dg != nil {
				return dg
			}
			if existing != nil {
				return diag.Errorf("cannot change interface to %q: an item for that interface already exists at %s (id %s)",
					newInterface, path, existing.GetID(routeros.Id))
			}
		}
		d.SetId(item.GetID(routeros.Id))
		dg = update(ctx, d, m)
		if dg.HasError() {
			return dg
		}
		d.SetId(newInterface)
		return dg
	}
}

func interfaceIdentityDelete(path string, del schema.DeleteContextFunc) schema.DeleteContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		item, dg := resolveByField(m, path, interfaceField, d.Id())
		if dg != nil {
			return dg
		}
		if item == nil {
			d.SetId("")
			return nil
		}
		if (*item)[defaultField] == routerTrue {
			iface := d.Id()
			d.SetId("")
			return diag.Diagnostics{{
				Severity: diag.Warning,
				Summary:  "default rule left on the router",
				Detail: fmt.Sprintf("the item for interface %q at %s is the router's default rule and cannot be removed; "+
					"it was released from management with its current settings left in place", iface, path),
			}}
		}
		d.SetId(item.GetID(routeros.Id))
		return del(ctx, d, m)
	}
}
