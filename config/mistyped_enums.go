package config

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// mistypedEnumFields are upstream schema fields typed as booleans for router
// properties that are not booleans. RouterOS 7.23 answers /ipv6/nd
// advertise-dns with self, yes or no and rejects true and false outright with
// HTTP 400 (verified) — self, which advertises the router's own address as the
// DNS server, being the setting the argument mostly exists for. Upstream's own
// example payload records "advertise-dns": "false", so the bool modelled a
// RouterOS that no longer exists.
//
// A bool cannot hold three states, and the damage runs past self being
// unsettable. BoolFromMikrotikJSON maps everything but true and yes to false,
// so a device holding self is observed as false while the schema's default of
// true puts true in the spec: observed never equals desired, and every
// reconcile writes advertise-dns=yes over the value the stanza exists for.
// This is the write-loop of the user group policy expansion and the system
// clock again, but neither of those cures reaches it — there is no observed
// value to normalise to, because the type itself is a state too small. Only a
// string can carry the router's own vocabulary in both directions.
//
// advertise-dns was found by hand; the rest of the list was not. hack/typedump
// asks a live router what every console property accepts (/console/inspect
// answers request=completion with the vocabulary) and
// hack/schemaaudit/mistyped.py diffs that against the upstream schema, which
// makes "a bool cannot hold three states" a query rather than a hunch. These
// seven are every field on 7.23.2 where it holds. Re-running the pair on a
// newer release is how this list stays honest, and both the manual and the
// router agree on the current set.
//
// Two of them are not the advertise-dns shape, and neither is a write loop:
//
//   - digest_algorithm is computed upstream and nothing else, so the provider
//     never sends it and the corruption is confined to the observation, which
//     reads off as false against a router holding sha256. It is in the list
//     because atProvider is an interface this provider ships too. The router
//     would refuse a boolean outright — 400 "input does not match any value
//     of digest-algorithm" to yes and no alike, verified — but no path in the
//     schema can offer it one.
//
//   - loop_protect_status is the inverse, and the worse of the two. The
//     router treats it as read-only and answers 400 "unknown parameter
//     loop-protect-status" to any write at all (verified), yet upstream marks
//     it optional and leaves it out of the serializer's skip list, so it sits
//     in spec.forProvider where a spec can set it and late-initialization can
//     fill it. Retyping fixes what the field observes — off, on or disabled
//     rather than a bool — and does not make it writable: a spec that carries
//     it still fails. Modelling it as computed-only is the other half of that
//     fix and is deliberately not attempted here.
//
// Dropping the default matters independently of the type: the serializer emits
// every defaulted field, so a spec that never mentions advertise-dns would
// still write yes on every request. Cleared here for the same reason
// withoutPhantomDefaults clears its own — that list is for arguments the
// router does not have at all, which is not this.
//
// Unlike the identity overrides this changes a generated CRD field's type, so
// it is applied to the generation schema as well as the runtime one: the CRD's
// string has to deserialize into a runtime string. Accepted values are not
// validated in the schema because upjet never runs ValidateFunc — the router
// rejects a bad value loudly instead, which is the same bargain
// withoutPhantomDefaults strikes.
var mistypedEnumFields = map[string][]string{
	"routeros_interface_macvlan":       {"loop_protect_status"},
	"routeros_interface_ovpn_client":   {"use_peer_dns"},
	"routeros_interface_sstp_client":   {"pfs"},
	"routeros_interface_sstp_server":   {"pfs"},
	"routeros_ipv6_dhcp_server":        {"use_radius"},
	"routeros_ipv6_neighbor_discovery": {"advertise_dns"},
	"routeros_system_certificate":      {"digest_algorithm"},
}

// enumValueDocs are appended to the retyped field's description so the
// generated CRD carries the vocabulary the schema can no longer express.
// Keyed by field name: the vocabulary belongs to the router property, so the
// two pfs fields share one entry.
var enumValueDocs = map[string]string{
	"advertise_dns": " RouterOS accepts \"self\" (advertise the router's own address), \"yes\" or \"no\"; " +
		"it rejects true and false.",
	"digest_algorithm": " RouterOS accepts \"md5\", \"sha1\", \"sha256\", \"sha384\" or \"sha512\"; " +
		"it rejects true, false, yes and no.",
	"loop_protect_status": " Read-only. RouterOS reports \"on\", \"off\" or \"disabled\".",
	"pfs": " RouterOS accepts \"required\" (demand perfect forward secrecy), \"yes\" or \"no\"; " +
		"it rejects true and false.",
	"use_peer_dns": " RouterOS accepts \"exclusively\" (use only the peer's servers), \"yes\" or \"no\"; " +
		"it rejects true and false.",
	"use_radius": " RouterOS accepts \"accounting\" (RADIUS for accounting only), \"yes\" or \"no\"; " +
		"it rejects true and false.",
}

func withoutMistypedEnums(p *schema.Provider) *schema.Provider {
	for resource, fields := range mistypedEnumFields {
		r := p.ResourcesMap[resource]
		if r == nil {
			// A rename upstream would otherwise surface as a nil
			// dereference partway through provider construction.
			panic("mistypedEnumFields: no upstream resource " + resource)
		}
		for _, field := range fields {
			s := r.Schema[field]
			if s == nil {
				panic("mistypedEnumFields: " + resource + " has no field " + field)
			}
			s.Type = schema.TypeString
			s.Default = nil
			s.Description += enumValueDocs[field]
		}
	}
	return p
}
