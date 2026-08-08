package config

import (
	_ "embed"

	ujconfig "github.com/crossplane/upjet/v2/pkg/config"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"

	"github.com/sindrip/provider-routeros/internal/routerosruntime"
)

const (
	resourcePrefix = "routeros"
	modulePath     = "github.com/sindrip/provider-routeros"
)

//go:embed schema.json
var providerSchema string

//go:embed provider-metadata.yaml
var providerMetadata string

// GetProvider returns the cluster-scoped provider configuration used for code
// generation. Generation uses a mechanically adapted copy of the upstream
// schema so Terraform names that Kubernetes cannot represent remain available
// through valid JSON field names and their original Terraform struct tags.
func GetProvider() *ujconfig.Provider {
	return newProvider("routeros.sindrip.io", false, providerForGeneration())
}

// GetProviderNamespaced returns the namespaced provider configuration used for
// code generation.
func GetProviderNamespaced() *ujconfig.Provider {
	return newProvider("routeros.m.sindrip.io", true, providerForGeneration())
}

// GetProviderRuntime returns the cluster-scoped provider configuration used by
// the controller. The runtime uses the upstream schemas unchanged except for
// the injected router-name fields (which generation also carries) and the
// name-identity overrides, which only affect CRUD behavior, not the
// generated APIs.
func GetProviderRuntime() *ujconfig.Provider {
	return newProvider("routeros.sindrip.io", false, routerosruntime.WrapProvider(providerForRuntime()))
}

// GetProviderNamespacedRuntime returns the namespaced runtime provider.
func GetProviderNamespacedRuntime() *ujconfig.Provider {
	return newProvider("routeros.m.sindrip.io", true, routerosruntime.WrapProvider(providerForRuntime()))
}

func providerForRuntime() *schema.Provider {
	return withCommentSequence(withFactoryIdentity(withCommentIdentity(withNameIdentity(injectRouterComment(injectRouterName(withoutPolicyExpansion(withoutPhantomDefaults(routeros.Provider()))))))))
}

// kindOverrides replaces derived Kubernetes kinds that Terraform names alone
// would make invalid or hazardous. "List" is the core v1 list wrapper and
// cannot be routed at all; "Service" and "Secret" shadow core kinds for
// kubectl and kind-keyed tooling; "Configuration" shadows Crossplane's
// package kind. The rest are readability fixes for acronym-heavy names.
var kindOverrides = map[string]string{
	"routeros_interface_6to4":        "SixToFour",
	"routeros_interface_list":        "InterfaceList",
	"routeros_interface_list_member": "InterfaceListMember",
	"routeros_ip_service":            "IPService",
	"routeros_ppp_secret":            "PPPSecret",
	"routeros_capsman_configuration": "CAPsMANConfiguration",
	"routeros_wifi_configuration":    "WifiConfiguration",
	"routeros_capsman_interface":     "CAPsMANInterface",
	"routeros_queue_type":            "QueueType",
	"routeros_zerotier_interface":    "ZeroTierInterface",
	"routeros_zerotier_controller":   "ZeroTierController",
}

func newProvider(rootGroup string, namespaced bool, terraformProvider *schema.Provider) *ujconfig.Provider {
	opts := []ujconfig.ProviderOption{
		ujconfig.WithRootGroup(rootGroup),
		ujconfig.WithIncludeList([]string{}),
		ujconfig.WithTerraformPluginSDKIncludeList([]string{".+"}),
		ujconfig.WithTerraformProvider(terraformProvider),
		ujconfig.WithFeaturesPackage("internal/features"),
		ujconfig.WithDefaultResourceOptions(
			ExternalNameConfiguration(),
			func(r *ujconfig.Resource) {
				if r.Name == "routeros_ip_dhcp_server_option_set" {
					// Upstream retains this singular resource as an alias for
					// routeros_ip_dhcp_server_option_sets. Both default to the
					// same Kubernetes plural, so the alias needs a unique path.
					r.Path = "dhcpserveroptionsetaliases"
				}
				if r.Name == "routeros_system_clock" {
					// date and time are readings that advance on their own, and
					// RouterOS excludes local time from config export on purpose.
					// Late-initializing them freezes a stale instant into desired
					// state, after which the reconciler writes the clock backwards
					// on every pass -- drifting real time and wedging the NTP
					// client, which cannot converge against something that keeps
					// overwriting it. A field that advances on its own must never
					// be late-initialized.
					r.LateInitializer.IgnoredFields = []string{"date", "time"}
				}
				if kind, ok := kindOverrides[r.Name]; ok {
					r.Kind = kind
				}
			},
		),
	}
	if namespaced {
		opts = append(opts, ujconfig.WithExampleManifestConfiguration(ujconfig.ExampleManifestConfiguration{
			ManagedResourceNamespace: "crossplane-system",
		}))
	}

	p := ujconfig.NewProvider([]byte(providerSchema), resourcePrefix, modulePath, []byte(providerMetadata), opts...)
	p.ConfigureResources()
	return p
}

func providerForGeneration() *schema.Provider {
	p := documentCommentSequence(injectRouterComment(injectRouterName(routeros.Provider())))
	renameFieldForGeneration(p, "routeros_ipv6_nd_prefix", "6to4_interface", "six_to_four_interface")
	renameFieldForGeneration(p, "routeros_wifi_interworking", "3gpp_info", "three_gpp_info")
	renameFieldForGeneration(p, "routeros_wifi_interworking", "3gpp_raw", "three_gpp_raw")
	return p
}

func renameFieldForGeneration(p *schema.Provider, resource, terraformName, generatedName string) {
	s := p.ResourcesMap[resource].Schema[terraformName]
	delete(p.ResourcesMap[resource].Schema, terraformName)
	s.Description += "\n+upjet:crd:field:TFTag=" + terraformName + ",omitempty"
	p.ResourcesMap[resource].Schema[generatedName] = s
}
