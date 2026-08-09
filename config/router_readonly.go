package config

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// routerReadOnlyFields are properties upstream offers as writable that the
// router will not accept as write parameters at all.
//
// loop-protect-status is the case this was written for. RouterOS reports it on
// every interface that has loop protection and answers 400 "unknown parameter
// loop-protect-status" to any attempt to set it (verified) — it is not an
// argument of add or set, and hack/typedump cannot even reach it through the
// add/set walk; it turns up only in the read-only pass, which enumerates a
// menu's properties from its print filter. Upstream marks it Optional anyway
// and leaves it out of the serializer's skip list, so it lands in
// spec.forProvider, where a manifest can set it and late-initialization can
// fill it from the observation. Either way the next update carries a parameter
// the router refuses and the resource stops reconciling.
//
// Marking it Computed and not Optional puts it where the router already has
// it: readable, never sent. Upjet then generates it into status.atProvider
// alone, so the CRD stops offering a field that could only ever fail.
//
// This is the second half of the v0.23.0 fix for the same field. The retype in
// mistypedFields corrected what the observation carries — off, on or disabled
// rather than a bool — and could not make the field writable, which was never
// the schema's to grant. Applied to the generation schema as well as the
// runtime one, because moving a field out of spec is a CRD change.
var routerReadOnlyFields = map[string][]string{
	"routeros_interface_macvlan": {"loop_protect_status"},
}

func withRouterReadOnly(p *schema.Provider) *schema.Provider {
	for resource, fields := range routerReadOnlyFields {
		r := p.ResourcesMap[resource]
		if r == nil {
			panic("routerReadOnlyFields: no upstream resource " + resource)
		}
		for _, field := range fields {
			s := r.Schema[field]
			if s == nil {
				panic("routerReadOnlyFields: " + resource + " has no field " + field)
			}
			s.Computed = true
			s.Optional = false
			s.Required = false
			s.Default = nil
			s.ForceNew = false
		}
	}
	return p
}
