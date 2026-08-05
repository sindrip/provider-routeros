package config

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

// nameIdentityResources are resources whose RouterOS name is enforced unique,
// so the name is a stable identifier while the internal .id is not: RouterOS
// reassigns .id when an item is recreated outside the controller. Switching
// the upstream ___id___ meta default to Name makes the upstream CRUD identify
// these items by name (resolving the current .id per operation), which also
// becomes the external-name annotation. The name is then effectively the
// identifier: changing the spec name renames the item on the router but
// strands the managed resource until its external-name annotation is updated
// to match.
var nameIdentityResources = []string{
	"routeros_interface_vlan",
	"routeros_system_user",
	"routeros_system_user_group",
}

func withNameIdentity(p *schema.Provider) *schema.Provider {
	for _, name := range nameIdentityResources {
		p.ResourcesMap[name].Schema[routeros.MetaId].Default = int(routeros.Name)
	}
	return p
}
