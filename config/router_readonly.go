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
// vrf on /ip/address is the second case. An address does not choose its VRF;
// it inherits the one its interface was put in through /ip/vrf, and the router
// reports the result back on the address. config/arg-types.json has it from the
// router itself: on /ip/address vrf is access read-only, enumerated by print
// rather than by the add/set walk — unlike the thirty-odd menus that do take
// vrf as a set argument. Upstream offers it as PropVrfRw all the same, so it
// lands in spec.forProvider, late-initialization fills it with main from the
// observation, and every create from that spec afterwards carries a parameter
// /ip/address/add will not accept.
var routerReadOnlyFields = map[string][]string{
	"routeros_interface_macvlan": {"loop_protect_status"},
	"routeros_ip_address":        {"vrf"},
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
			// Upstream hands several resources the same *schema.Schema value —
			// PropVrfRw is one pointer shared by every menu that takes vrf as a
			// set argument. Retyping it in place would follow the pointer out to
			// all of them and take a writable field away from menus the router
			// lets write it. Demote a copy, and leave the shared one alone.
			demoted := *s
			demoted.Computed = true
			demoted.Optional = false
			demoted.Required = false
			demoted.Default = nil
			demoted.ForceNew = false
			r.Schema[field] = &demoted
		}
	}
	return p
}
