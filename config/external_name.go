package config

import ujconfig "github.com/crossplane/upjet/v2/pkg/config"

var omittedRouterOSFields = []string{
	"___id___",
	"___path___",
	"___ts___",
	"___skip___",
	"___unset___",
	"___drop_val___",
}

// ExternalNameConfiguration preserves the identifier returned by the official
// Terraform provider. It deliberately does not infer names or implement local
// import/adoption behavior.
func ExternalNameConfiguration() ujconfig.ResourceOption {
	return func(r *ujconfig.Resource) {
		e := ujconfig.IdentifierFromProvider
		e.OmittedFields = append([]string(nil), omittedRouterOSFields...)
		r.ExternalName = e
	}
}
