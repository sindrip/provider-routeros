package config

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// phantomDefaultFields are upstream schema fields for arguments the router
// does not have — pinned in config/console-tree.json and verified by live
// probe, where REST answers "unknown parameter" — but that carry an SDK
// default in the upstream schema. The upstream serializer treats a defaulted
// field as always present, so every create sends the argument and the router
// rejects the whole request: create is impossible no matter what the
// configuration says. Dropping the default keeps the field out of requests
// unless it is explicitly set, which the router still rejects — loudly, as
// it should for an argument it does not know. The arguments RouterOS
// actually has for add-path (input.add-path, output.add-path) are an
// upstream modeling gap, not a rename of this field.
//
// Membership is judged at the serializer, not the schema: address_families
// also looks phantom next to the console tree, but upstream's per-version
// drift table rewrites it to afi (RouterOS >= 7.19) before sending, so only
// fields the serializer emits under a router-unknown name belong here.
var phantomDefaultFields = map[string][]string{
	"routeros_routing_bgp_connection": {"add_path_out"},
	"routeros_routing_bgp_template":   {"add_path_out"},
}

func withoutPhantomDefaults(p *schema.Provider) *schema.Provider {
	for name, fields := range phantomDefaultFields {
		for _, field := range fields {
			p.ResourcesMap[name].Schema[field].Default = nil
		}
	}
	return p
}
