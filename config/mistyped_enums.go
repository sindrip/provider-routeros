package config

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// mistypedField is one field and the router vocabulary its Terraform type
// cannot hold.
//
// upstream records what the schema models it as today, and the gate test fails
// if that changes: a fix landing upstream should retire the entry rather than
// have this quietly shadow it. doc hangs off the entry rather than off the
// field name because the same name carries different vocabularies on different
// menus — a DHCP client option's code takes four names, a server option's
// takes one.
type mistypedField struct {
	field    string
	upstream schema.ValueType
	doc      string
}

// Vocabularies shared by more than one entry. The value belongs to the router
// property, so every menu exposing it says the same thing.
const (
	docPFS = " RouterOS accepts \"required\" (demand perfect forward secrecy), \"yes\" or \"no\"; " +
		"it rejects true and false."
	docNewDSCP = " RouterOS accepts a DSCP number, or \"from-priority\" or " +
		"\"from-priority-to-high-3-bits\"."
	docVLANID         = " RouterOS accepts a VLAN id 1..4095, or \"none\"; 0 is out of range."
	docClientMACLimit = " RouterOS accepts a number of clients, or \"unlimited\" (its default). " +
		"Note that 0 is a limit of zero, not unlimited."
	docClientOptionCode = " RouterOS accepts a DHCP option code 1..254, or \"client-id\", " +
		"\"hostname\", \"vendor-class-id\" or \"vendor-specific\"; 0 is out of range."
	docServerOptionCode = " RouterOS accepts a DHCP option code 1..254, or \"vendor-specific\"; " +
		"0 is out of range."
)

// mistypedFields are upstream schema fields typed as booleans or numbers for
// router properties that are neither. RouterOS 7.23 answers /ipv6/nd
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
// makes "a bool cannot hold three states" a query rather than a hunch. Seven
// bools on 7.23.2 answer it. Re-running the pair on a newer release is how
// this list stays honest, and both the manual and the router agree on the set.
//
// The same query asked of the numbers finds fourteen more, where the router
// also answers with a word: unlimited, from-priority, vendor-specific, none.
// Those divide by what the word costs. Where the router refuses the number the
// int would send — vlan-id is 1..4095 and code is 1..254, so neither reaches 0
// — a row using the word cannot be managed at all, because every write of the
// observed value is rejected. Where it accepts it, the loss is quieter and
// worse: a DHCP server holding client-mac-limit=unlimited is observed as 0, 0
// is a real limit of zero, and writing it back silently reconfigures the
// device and then sits still, with nothing left to notice.
//
// Four numeric fields the sweep raised are deliberately absent, and belong
// here so they are not "found" again: hop-limit and mtu on /ipv6/nd, and
// keepalive-timeout on the l2tp and pppoe clients. Their sentinel is a
// spelling of zero — writing 0 to hop-limit stores unspecified, and to
// keepalive-timeout stores disabled (both verified) — so the int round-trips
// exactly and only atProvider reads a little oddly. Retyping them would cost
// every affected user the status migration in the v0.23.0 release note and buy
// nothing.
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
//     rather than a bool — and cannot make it writable, which was never the
//     schema's to grant. withRouterReadOnly is the other half: it marks the
//     field computed-only, so nothing puts it in spec to begin with.
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
var mistypedFields = map[string][]mistypedField{
	// Booleans over a third state (v0.23.0).
	"routeros_interface_macvlan": {{"loop_protect_status", schema.TypeBool,
		" Read-only. RouterOS reports \"on\", \"off\" or \"disabled\"."}},
	"routeros_interface_ovpn_client": {{"use_peer_dns", schema.TypeBool,
		" RouterOS accepts \"exclusively\" (use only the peer's servers), \"yes\" or \"no\"; " +
			"it rejects true and false."}},
	"routeros_interface_sstp_client": {{"pfs", schema.TypeBool, docPFS}},
	"routeros_interface_sstp_server": {{"pfs", schema.TypeBool, docPFS}},
	"routeros_ipv6_dhcp_server": {{"use_radius", schema.TypeBool,
		" RouterOS accepts \"accounting\" (RADIUS for accounting only), \"yes\" or \"no\"; " +
			"it rejects true and false."}},
	"routeros_ipv6_neighbor_discovery": {{"advertise_dns", schema.TypeBool,
		" RouterOS accepts \"self\" (advertise the router's own address), \"yes\" or \"no\"; " +
			"it rejects true and false."}},
	"routeros_system_certificate": {{"digest_algorithm", schema.TypeBool,
		" RouterOS accepts \"md5\", \"sha1\", \"sha256\", \"sha384\" or \"sha512\"; " +
			"it rejects true, false, yes and no."}},

	// Numbers over a sentinel word (v0.24.0).
	"routeros_dhcp_client_option":            {{"code", schema.TypeInt, docClientOptionCode}},
	"routeros_ip_dhcp_client_option":         {{"code", schema.TypeInt, docClientOptionCode}},
	"routeros_ip_dhcp_server_option":         {{"code", schema.TypeInt, docServerOptionCode}},
	"routeros_ip_dhcp_server_option_matcher": {{"code", schema.TypeInt, docServerOptionCode}},
	"routeros_dhcp_server":                   {{"client_mac_limit", schema.TypeInt, docClientMACLimit}},
	"routeros_ip_dhcp_server":                {{"client_mac_limit", schema.TypeInt, docClientMACLimit}},
	"routeros_firewall_mangle":               {{"new_dscp", schema.TypeInt, docNewDSCP}},
	"routeros_ip_firewall_mangle":            {{"new_dscp", schema.TypeInt, docNewDSCP}},
	"routeros_interface_bridge_filter": {
		{"new_priority", schema.TypeInt, " RouterOS accepts a priority number, or \"from-ingress\"."},
		{"vlan_encap", schema.TypeInt, " RouterOS accepts an EtherType 0x0000..0xFFFF, or a protocol " +
			"name such as \"arp\", \"ip\", \"ipv6\" or \"vlan\"."},
	},
	"routeros_interface_l2tp_client": {{"l2tpv3_cookie_length", schema.TypeInt,
		" RouterOS accepts \"0\", \"4-bytes\" or \"8-bytes\"."}},
	"routeros_wifi_access_list":               {{"vlan_id", schema.TypeInt, docVLANID}},
	"routeros_wifi_datapath":                  {{"vlan_id", schema.TypeInt, docVLANID}},
	"routeros_wifi_security_multi_passphrase": {{"vlan_id", schema.TypeInt, docVLANID}},
}

func withoutMistypedEnums(p *schema.Provider) *schema.Provider {
	for resource, fields := range mistypedFields {
		r := p.ResourcesMap[resource]
		if r == nil {
			// A rename upstream would otherwise surface as a nil
			// dereference partway through provider construction.
			panic("mistypedFields: no upstream resource " + resource)
		}
		for _, f := range fields {
			s := r.Schema[f.field]
			if s == nil {
				panic("mistypedFields: " + resource + " has no field " + f.field)
			}
			s.Type = schema.TypeString
			s.Default = nil
			s.Description += f.doc
		}
	}
	return p
}
