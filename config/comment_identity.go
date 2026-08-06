package config

import (
	"context"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// commentIdentityResources are resources whose RouterOS items have no name and
// no other enforced-unique field, so the ephemeral internal .id would be the
// only identity — and RouterOS reassigns it when an item is deleted and
// recreated, which permanently breaks reconciliation (and makes Create with a
// stale id silently mint duplicates). The comment becomes the identity
// instead: required at create, enforced unique by this provider (RouterOS
// does not enforce it), and resolved to the current .id on every operation.
// Rule ordering is out of scope here — see docs/adr/0001; this is the interim
// measure it describes.
var commentIdentityResources = []string{
	"routeros_bridge_port",
	"routeros_bridge_vlan",
	"routeros_firewall_nat",
	"routeros_interface_bridge_port",
	"routeros_interface_bridge_vlan",
	"routeros_ip_firewall_nat",
}

const commentField = "comment"

func withCommentIdentity(p *schema.Provider) *schema.Provider {
	for _, name := range commentIdentityResources {
		r := p.ResourcesMap[name]
		path, _ := r.Schema[routeros.MetaResourcePath].Default.(string)
		r.CreateContext = commentIdentityCreate(path, r.CreateContext)
		r.ReadContext = commentIdentityRead(path, r.ReadContext)
		r.UpdateContext = commentIdentityUpdate(path, r.UpdateContext)
		r.DeleteContext = commentIdentityDelete(path, r.DeleteContext)
	}
	return p
}

func commentIdentityCreate(path string, create schema.CreateContextFunc) schema.CreateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		comment := d.Get(commentField).(string)
		if comment == "" {
			return diag.Errorf("comment is required: items at %s have no name, so the comment is the resource identity", path)
		}
		item, dg := resolveByComment(m, path, comment)
		if dg != nil {
			return dg
		}
		if item != nil {
			return diag.Errorf("an item with comment %q already exists at %s (id %s): the comment is the resource identity and must be unique", comment, path, item.GetID(routeros.Id))
		}
		dg = create(ctx, d, m)
		if dg.HasError() {
			return dg
		}
		d.SetId(comment)
		return dg
	}
}

func commentIdentityRead(path string, read schema.ReadContextFunc) schema.ReadContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		item, dg := resolveByComment(m, path, d.Id())
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
			d.SetId(d.Get(commentField).(string))
		}
		return dg
	}
}

func commentIdentityUpdate(path string, update schema.UpdateContextFunc) schema.UpdateContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		newComment := d.Get(commentField).(string)
		if newComment == "" {
			return diag.Errorf("comment cannot be cleared: it is the identity of items at %s", path)
		}
		item, dg := resolveByComment(m, path, d.Id())
		if dg != nil {
			return dg
		}
		if item == nil {
			return diag.Errorf("item with comment %q no longer exists at %s", d.Id(), path)
		}
		if newComment != d.Id() {
			if dg := rejectExisting(m, path, newComment); dg != nil {
				return dg
			}
		}
		d.SetId(item.GetID(routeros.Id))
		dg = update(ctx, d, m)
		if dg.HasError() {
			return dg
		}
		d.SetId(newComment)
		return dg
	}
}

func commentIdentityDelete(path string, del schema.DeleteContextFunc) schema.DeleteContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		item, dg := resolveByComment(m, path, d.Id())
		if dg != nil {
			return dg
		}
		if item == nil {
			d.SetId("")
			return nil
		}
		d.SetId(item.GetID(routeros.Id))
		return del(ctx, d, m)
	}
}

// resolveByComment returns the single item carrying the comment, nil when the
// comment matches nothing, and an error diagnostic when the lookup fails or
// several items share the comment.
func resolveByComment(m any, path, comment string) (*routeros.MikrotikItem, diag.Diagnostics) {
	items, err := itemsByComment(m, path, comment)
	if err != nil {
		return nil, diag.FromErr(err)
	}
	switch len(items) {
	case 0:
		return nil, nil
	case 1:
		return &items[0], nil
	default:
		return nil, diag.Errorf("identity is ambiguous: %d items at %s share comment %q; make comments unique before managing them", len(items), path, comment)
	}
}

func rejectExisting(m any, path, comment string) diag.Diagnostics {
	items, err := itemsByComment(m, path, comment)
	if err != nil {
		return diag.FromErr(err)
	}
	if len(items) > 0 {
		return diag.Errorf("cannot change comment to %q: an item with that comment already exists at %s (id %s)", comment, path, items[0].GetID(routeros.Id))
	}
	return nil
}

func itemsByComment(m any, path, comment string) ([]routeros.MikrotikItem, error) {
	c := m.(routeros.Client)
	filter := comment
	if c.GetTransport() == routeros.TransportREST {
		// Percent-encode for the query string. RouterOS matches %20 as a
		// space but treats '+' literally, so QueryEscape alone is wrong
		// (verified against 7.23.2).
		filter = strings.ReplaceAll(url.QueryEscape(comment), "+", "%20")
	}
	res, err := routeros.ReadItemsFiltered([]string{commentField + "=" + filter}, path, c)
	if err != nil {
		return nil, err
	}
	return *res, nil
}
