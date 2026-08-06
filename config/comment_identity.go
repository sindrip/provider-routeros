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
	"routeros_firewall_nat",
	"routeros_ip_firewall_nat",
}

func withCommentIdentity(p *schema.Provider) *schema.Provider {
	for _, name := range commentIdentityResources {
		wrapCommentIdentity(p.ResourcesMap[name])
	}
	return p
}

func wrapCommentIdentity(r *schema.Resource) {
	path, _ := r.Schema[routeros.MetaResourcePath].Default.(string)
	create, read, update, del := r.CreateContext, r.ReadContext, r.UpdateContext, r.DeleteContext

	r.CreateContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		comment := d.Get("comment").(string)
		if comment == "" {
			return diag.Errorf("comment is required: items at %s have no name, so the comment is the resource identity", path)
		}
		items, err := itemsByComment(m, path, comment)
		if err != nil {
			return diag.FromErr(err)
		}
		if len(items) > 0 {
			return diag.Errorf("an item with comment %q already exists at %s (id %s): the comment is the resource identity and must be unique", comment, path, items[0].GetID(routeros.Id))
		}
		dg := create(ctx, d, m)
		if dg.HasError() {
			return dg
		}
		d.SetId(comment)
		return dg
	}

	r.ReadContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		items, err := itemsByComment(m, path, d.Id())
		if err != nil {
			return diag.FromErr(err)
		}
		if len(items) == 0 {
			d.SetId("")
			return nil
		}
		if len(items) > 1 {
			return ambiguous(path, d.Id(), len(items))
		}
		d.SetId(items[0].GetID(routeros.Id))
		dg := read(ctx, d, m)
		if dg.HasError() {
			return dg
		}
		if d.Id() != "" {
			d.SetId(d.Get("comment").(string))
		}
		return dg
	}

	r.UpdateContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		newComment := d.Get("comment").(string)
		if newComment == "" {
			return diag.Errorf("comment cannot be cleared: it is the identity of items at %s", path)
		}
		items, err := itemsByComment(m, path, d.Id())
		if err != nil {
			return diag.FromErr(err)
		}
		if len(items) == 0 {
			return diag.Errorf("item with comment %q no longer exists at %s", d.Id(), path)
		}
		if len(items) > 1 {
			return ambiguous(path, d.Id(), len(items))
		}
		if newComment != d.Id() {
			dupes, err := itemsByComment(m, path, newComment)
			if err != nil {
				return diag.FromErr(err)
			}
			if len(dupes) > 0 {
				return diag.Errorf("cannot change comment to %q: an item with that comment already exists at %s (id %s)", newComment, path, dupes[0].GetID(routeros.Id))
			}
		}
		d.SetId(items[0].GetID(routeros.Id))
		dg := update(ctx, d, m)
		if dg.HasError() {
			return dg
		}
		d.SetId(newComment)
		return dg
	}

	r.DeleteContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		items, err := itemsByComment(m, path, d.Id())
		if err != nil {
			return diag.FromErr(err)
		}
		if len(items) == 0 {
			d.SetId("")
			return nil
		}
		if len(items) > 1 {
			return ambiguous(path, d.Id(), len(items))
		}
		d.SetId(items[0].GetID(routeros.Id))
		return del(ctx, d, m)
	}
}

func ambiguous(path, comment string, n int) diag.Diagnostics {
	return diag.Errorf("identity is ambiguous: %d items at %s share comment %q; make comments unique before managing them", n, path, comment)
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
	res, err := routeros.ReadItemsFiltered([]string{"comment=" + filter}, path, c)
	if err != nil {
		return nil, err
	}
	return *res, nil
}
