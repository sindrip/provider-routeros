package config

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// withCommentSequence rewires routeros_move_items — the sequencer for
// order-sensitive menus — to accept comments in the sequence instead of the
// ephemeral RouterOS .id values the upstream resource wants. Upstream
// semantics are kept: the last entry is the anchor, everything before it is
// placed ahead of it in listed order by a single /move call, and read reports
// the managed rows in device order so a reorder shows up as drift. Each
// operation resolves the comments to the rows' current ids, so the sequence
// survives id churn the same way comment identity does. Desired order lives
// only in the spec; nothing position-like is written to the router. One Items
// resource per menu is the single owner of that menu's order — see
// docs/adr/0002 for why ordering must not live on the rule resources.
//
// Entries beginning with '*' are passed through as literal .id values
// (RouterOS ids always carry that prefix, comments should not), which keeps
// raw-id sequences working.

const (
	moveItemsResource = "routeros_move_items"
	sequenceField     = "sequence"
	resourcePathField = "resource_path"
	resourceNameField = "resource_name"
)

func withCommentSequence(p *schema.Provider) *schema.Provider {
	r := p.ResourcesMap[moveItemsResource]
	r.CreateContext = commentSequenceWrite(r.CreateContext)
	r.UpdateContext = commentSequenceWrite(r.UpdateContext)
	r.ReadContext = commentSequenceRead(r.ReadContext)
	return documentCommentSequence(p)
}

// documentCommentSequence rewrites the upstream field documentation to match
// the comment-addressed semantics withCommentSequence gives the runtime; the
// generation schema applies it too so the CRDs carry the real contract.
func documentCommentSequence(p *schema.Provider) *schema.Provider {
	p.ResourcesMap[moveItemsResource].Schema[sequenceField].Description = "Rows of the menu in the desired order, each identified by its comment" +
		" (an entry starting with `*` is taken as a literal RouterOS id instead). The last entry is the anchor: it is never moved, and all preceding" +
		" entries are placed before it in the listed order. Every comment must resolve to exactly one existing row before the sequence can be applied;" +
		" rows not listed are left where they are."
	return p
}

// crudFunc is the shared shape of the SDK's Create/Read/UpdateContextFunc.
type crudFunc = func(context.Context, *schema.ResourceData, any) diag.Diagnostics

func commentSequenceWrite(inner crudFunc) crudFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		entries := sequenceEntries(d)
		if len(entries) < 2 {
			return diag.Errorf("sequence needs at least two entries: the last is the anchor the others are placed before")
		}
		ids, back, dg := resolveSequence(m, menuPath(d), entries, false)
		if dg != nil {
			return dg
		}
		if err := setSequence(d, ids); err != nil {
			return diag.FromErr(err)
		}
		dg = inner(ctx, d, m)
		if err := restoreSequence(d, back); err != nil && !dg.HasError() {
			return diag.FromErr(err)
		}
		return dg
	}
}

func commentSequenceRead(inner schema.ReadContextFunc) schema.ReadContextFunc {
	return func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		// Lenient: a row deleted out of band must surface as drift, not as a
		// failing read; its own resource will recreate it and the resulting
		// update converges the order.
		ids, back, dg := resolveSequence(m, menuPath(d), sequenceEntries(d), true)
		if dg != nil {
			return dg
		}
		if err := setSequence(d, ids); err != nil {
			return diag.FromErr(err)
		}
		if dg := inner(ctx, d, m); dg.HasError() || d.Id() == "" {
			return dg
		}
		if err := restoreSequence(d, back); err != nil {
			return diag.FromErr(err)
		}
		return nil
	}
}

// menuPath mirrors the upstream resource's path derivation, normalized to a
// leading slash so the REST lookup URL is well-formed either way.
func menuPath(d *schema.ResourceData) string {
	if p, ok := d.GetOk(resourcePathField); ok {
		return p.(string)
	}
	name := d.Get(resourceNameField).(string)
	return "/" + strings.ReplaceAll(strings.TrimPrefix(name, resourcePrefix+"_"), "_", "/")
}

// resolveSequence maps sequence entries to the rows' current ids in one menu
// read. Lenient mode drops entries whose comment matches nothing; strict mode
// makes them errors. A comment matching several rows is always an error: the
// sequence cannot be applied safely while the identity is ambiguous. The
// returned map translates ids back to the entries that produced them.
func resolveSequence(m any, path string, entries []string, lenient bool) ([]string, map[string]string, diag.Diagnostics) {
	res, err := routeros.ReadItems(nil, path, m.(routeros.Client))
	if err != nil {
		return nil, nil, diag.FromErr(err)
	}
	byComment := map[string][]string{}
	for _, item := range *res {
		if c := item[commentField]; c != "" {
			byComment[c] = append(byComment[c], item.GetID(routeros.Id))
		}
	}
	ids := make([]string, 0, len(entries))
	back := make(map[string]string, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if seen[e] {
			return nil, nil, diag.Errorf("sequence lists %q twice; each entry identifies one row", e)
		}
		seen[e] = true
		id := e
		if !strings.HasPrefix(e, "*") {
			switch matches := byComment[e]; len(matches) {
			case 1:
				id = matches[0]
			case 0:
				if lenient {
					continue
				}
				return nil, nil, diag.Errorf("no item with comment %q at %s: create the row (or wait for its resource to reconcile) before sequencing it", e, path)
			default:
				return nil, nil, diag.Errorf("identity is ambiguous: %d items at %s share comment %q; make them unique before sequencing them", len(matches), path, e)
			}
		}
		ids = append(ids, id)
		back[id] = e
	}
	return ids, back, nil
}

func sequenceEntries(d *schema.ResourceData) []string {
	raw := d.Get(sequenceField).([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		out = append(out, s)
	}
	return out
}

func setSequence(d *schema.ResourceData, ids []string) error {
	vals := make([]any, len(ids))
	for i, id := range ids {
		vals[i] = id
	}
	return d.Set(sequenceField, vals)
}

// restoreSequence rewrites the id sequence the upstream CRUD left in state
// back into the caller's entries, preserving the device order the read
// established so drift stays visible.
func restoreSequence(d *schema.ResourceData, back map[string]string) error {
	ids := sequenceEntries(d)
	out := make([]any, len(ids))
	for i, id := range ids {
		if e, ok := back[id]; ok {
			out[i] = e
		} else {
			out[i] = id
		}
	}
	return d.Set(sequenceField, out)
}
