package config

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// nameIdentityResources are resources whose RouterOS name is enforced unique,
// as verified by hack/uniqprobe against a disposable CHR — see
// config/name-uniqueness.json for the pinned per-resource verdicts. Only add
// resources here whose verdict is UNIQUE (never DUPLICATE, and UNTESTED only
// after probing on hardware that supports them) and whose upstream CRUD uses
// the generic Default*/Resource* helpers, which honor the ___id___ meta;
// resources with hand-written CRUD may ignore it. routing_table still does.
// system_certificate does not: its hand-written create and delete pass d.Id()
// straight through as a RouterOS number/numbers argument, and the router
// resolves a name there exactly as it resolves a *N (verified against
// 7.23.2), so the override reaches every one of its paths.
//
// Context: RouterOS reassigns the internal .id when an item is deleted and
// recreated outside the controller, which permanently broke reconciliation
// for managed resources whose external-name stored that ephemeral id.
// Switching the upstream ___id___ meta default to Name makes the upstream
// CRUD identify these items by name (resolving the current .id per
// operation), which also becomes the external-name annotation. The name is
// then effectively the identifier: changing the spec name renames the item
// on the router but strands the managed resource until its external-name
// annotation is updated to match.
var nameIdentityResources = []string{
	"routeros_bridge",
	"routeros_dhcp_client",
	"routeros_dhcp_client_option",
	"routeros_dhcp_server",
	"routeros_file",
	"routeros_gre",
	"routeros_interface_6to4",
	"routeros_interface_bonding",
	"routeros_interface_bridge",
	"routeros_interface_eoip",
	"routeros_interface_gre",
	"routeros_interface_gre6",
	"routeros_interface_ipip",
	"routeros_interface_l2tp_client",
	"routeros_interface_list",
	"routeros_interface_lte_apn",
	"routeros_interface_macvlan",
	"routeros_interface_ovpn_client",
	"routeros_interface_ovpn_server",
	"routeros_interface_pppoe_client",
	"routeros_interface_pppoe_server",
	"routeros_interface_sstp_client",
	"routeros_interface_vlan",
	"routeros_interface_vrrp",
	"routeros_interface_vxlan",
	"routeros_interface_wireguard",
	"routeros_interface_wireguard_peer",
	"routeros_ip_dhcp_client",
	"routeros_ip_dhcp_client_option",
	"routeros_ip_dhcp_server",
	"routeros_ip_dhcp_server_option",
	"routeros_ip_dhcp_server_option_matcher",
	"routeros_ip_dhcp_server_option_set",
	"routeros_ip_dhcp_server_option_sets",
	"routeros_ip_dns_forwarders",
	"routeros_ip_firewall_layer7_protocol",
	"routeros_ip_hotspot",
	"routeros_ip_hotspot_profile",
	"routeros_ip_hotspot_user",
	"routeros_ip_hotspot_user_profile",
	"routeros_ip_ipsec_mode_config",
	"routeros_ip_ipsec_peer",
	"routeros_ip_ipsec_policy_group",
	"routeros_ip_ipsec_profile",
	"routeros_ip_ipsec_proposal",
	"routeros_ip_pool",
	"routeros_ip_vrf",
	"routeros_ipip",
	"routeros_ipv6_dhcp_client_option",
	"routeros_ipv6_dhcp_server",
	"routeros_ipv6_dhcp_server_option",
	"routeros_ipv6_dhcp_server_option_sets",
	"routeros_ipv6_pool",
	"routeros_ppp_profile",
	"routeros_ppp_secret",
	"routeros_queue_simple",
	"routeros_queue_tree",
	"routeros_queue_type",
	"routeros_scheduler",
	"routeros_snmp_community",
	"routeros_system_certificate",
	"routeros_system_scheduler",
	"routeros_system_script",
	"routeros_system_user",
	"routeros_system_user_group",
	"routeros_vlan",
	"routeros_vrrp",
	"routeros_wifi_aaa",
	"routeros_wifi_channel",
	"routeros_wifi_configuration",
	"routeros_wifi_datapath",
	"routeros_wifi_interworking",
	"routeros_wifi_security",
	"routeros_wifi_steering",
	"routeros_wireguard",
	"routeros_wireguard_peer",
}

// injectedNameResources have a settable, enforced-unique name property on the
// router (see /ip/dhcp-client add in config/console-tree.json and the pinned
// UNIQUE verdicts) that the upstream Terraform schema does not model. The
// field is injected into both the generation schema (so the CRD carries
// spec.forProvider.name) and the runtime schema (so the upstream CRUD
// serializes it and can set the Terraform ID from it under Name identity).
// It is Required because upstream ResourceCreate derives the ID from the name
// in the request body; an omitted name would leave the ID empty and fail the
// create. If upstream ever models the field itself, the injection becomes
// redundant and the test will flag it.
var injectedNameResources = []string{
	"routeros_dhcp_client",
	"routeros_ip_dhcp_client",
}

func injectRouterName(p *schema.Provider) *schema.Provider {
	for _, name := range injectedNameResources {
		p.ResourcesMap[name].Schema["name"] = &schema.Schema{
			Type:        schema.TypeString,
			Required:    true,
			Description: "Name of the item on the router. RouterOS enforces uniqueness; the provider uses it as the resource identity.",
		}
	}
	return p
}

func withNameIdentity(p *schema.Provider) *schema.Provider {
	for _, name := range nameIdentityResources {
		p.ResourcesMap[name].Schema[routeros.MetaId].Default = int(routeros.Name)
	}
	return p
}
