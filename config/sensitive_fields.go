package config

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// sensitiveFields are upstream schema fields carrying key material that
// upstream does not mark Sensitive.
//
// The consequence is not only that a secret has to be written into the spec in
// cleartext. A field that is neither Sensitive nor Computed is generated into
// the observation as a plain string, and RouterOS answers a REST read with
// these values in cleartext — the wireguard peer's private key comes back in
// the row like any other property (verified). So the provider copies whatever
// key the router holds into status.atProvider, where it is readable by anyone
// with get on the managed resource, stored in etcd, and printed by kubectl get
// -o yaml. That happens whether or not a spec ever declared one.
//
// Upstream is inconsistent rather than uniformly wrong, which is what makes
// this a defect and not a design: on the wireguard peer itself preshared_key
// is Sensitive and the wireguard interface's own private_key is Sensitive,
// while the peer's private_key is not. The three are the same kind of secret.
//
// Marking them Sensitive is what upjet keys off to generate a
// SecretKeySelector instead of a string and to keep the value out of the
// observation, routing an observed secret to connection details instead. The
// wireguard interface is the working reference for the shape that produces.
//
// The list came from sweeping the schema for secret-shaped names and dropping
// what only reads like one: always_allow_password_login and
// minimum_password_length are settings, password_format is a format,
// multi_passphrase_group names a group, and the certificate's private_key is a
// bool saying whether a key is present rather than the key. Re-run the sweep
// on a schema bump the way hack/typedump's pair is re-run — a new resource
// carrying a password is the likely way this list goes stale.
//
// Applied to the generation schema as well as the runtime one: moving a field
// into a SecretKeySelector is a CRD change.
var sensitiveFields = map[string][]string{
	"routeros_capsman_access_list":                  {"private_passphrase"},
	"routeros_interface_lte_apn":                    {"password"},
	"routeros_interface_wireguard_peer":             {"private_key"},
	"routeros_interface_wireless_access_list":       {"private_key", "private_pre_shared_key"},
	"routeros_interface_wireless_security_profiles": {"mschapv2_password"},
	"routeros_ip_dhcp_server_config":                {"radius_password"},
	"routeros_user_manager_advanced":                {"paypal_password", "web_private_password"},
	"routeros_user_manager_user":                    {"password", "otp_secret"},
	"routeros_wifi_access_list":                     {"passphrase"},
	"routeros_wifi_security":                        {"passphrase", "eap_password"},
	"routeros_wireguard_peer":                       {"private_key"},
}

func withSensitiveFields(p *schema.Provider) *schema.Provider {
	for resource, fields := range sensitiveFields {
		r := p.ResourcesMap[resource]
		if r == nil {
			panic("sensitiveFields: no upstream resource " + resource)
		}
		for _, field := range fields {
			s := r.Schema[field]
			if s == nil {
				panic("sensitiveFields: " + resource + " has no field " + field)
			}
			s.Sensitive = true
		}
	}
	return p
}
