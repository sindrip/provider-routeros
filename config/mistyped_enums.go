package config

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// mistypedEnumFields are upstream schema fields typed as booleans for router
// arguments that are not booleans. RouterOS 7.23 answers /ipv6/nd
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
	"routeros_ipv6_neighbor_discovery": {"advertise_dns"},
}

// enumValueDocs are appended to the retyped field's description so the
// generated CRD carries the vocabulary the schema can no longer express.
var enumValueDocs = map[string]string{
	"advertise_dns": " RouterOS accepts \"self\" (advertise the router's own address), \"yes\" or \"no\"; " +
		"it rejects true and false.",
}

func withoutMistypedEnums(p *schema.Provider) *schema.Provider {
	for resource, fields := range mistypedEnumFields {
		for _, field := range fields {
			s := p.ResourcesMap[resource].Schema[field]
			s.Type = schema.TypeString
			s.Default = nil
			s.Description += enumValueDocs[field]
		}
	}
	return p
}
