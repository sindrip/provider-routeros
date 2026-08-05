package config

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/gobuffalo/flect"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/terraform-routeros/terraform-provider-routeros/routeros"
)

func TestProviderIncludesEveryUpstreamResource(t *testing.T) {
	upstream := routeros.Provider().ResourcesMap
	for name, configured := range map[string]map[string]struct{}{
		"cluster":    resourceNames(GetProvider().Resources),
		"namespaced": resourceNames(GetProviderNamespaced().Resources),
	} {
		if len(configured) != len(upstream) {
			t.Fatalf("%s provider configured %d resources, want %d", name, len(configured), len(upstream))
		}
		for resource := range upstream {
			if _, ok := configured[resource]; !ok {
				t.Errorf("%s provider is missing %s", name, resource)
			}
		}
	}
}

func TestProviderMechanicalAdaptations(t *testing.T) {
	p := GetProvider()
	wantKinds := map[string]string{
		"routeros_interface_6to4":      "SixToFour",
		"routeros_capsman_interface":   "CAPsMANInterface",
		"routeros_queue_type":          "QueueType",
		"routeros_zerotier_interface":  "ZeroTierInterface",
		"routeros_zerotier_controller": "ZeroTierController",
	}
	seenGVKs := map[string]string{}
	seenPaths := map[string]string{}
	for name, resource := range p.Resources {
		if want := wantKinds[name]; want != "" && resource.Kind != want {
			t.Errorf("%s kind = %s, want %s", name, resource.Kind, want)
		}
		gvk := resource.ShortGroup + "/" + resource.Version + "/" + resource.Kind
		gvkKey := strings.ToLower(gvk)
		if previous := seenGVKs[gvkKey]; previous != "" {
			t.Errorf("resources %s and %s share %s", previous, name, gvk)
		}
		seenGVKs[gvkKey] = name
		path := resource.Path
		if path == "" {
			path = strings.ToLower(flect.Pluralize(resource.Kind))
		}
		pathKey := resource.ShortGroup + "/" + path
		if previous := seenPaths[pathKey]; previous != "" {
			t.Errorf("resources %s and %s share Kubernetes path %s", previous, name, pathKey)
		}
		seenPaths[pathKey] = name
		for _, field := range omittedRouterOSFields {
			if !slices.Contains(resource.ExternalName.OmittedFields, field) {
				t.Errorf("%s does not omit internal field %s", name, field)
			}
		}
	}
	if got := p.Resources["routeros_ip_dhcp_server_option_set"].Path; got != "dhcpserveroptionsetaliases" {
		t.Errorf("legacy DHCP option set path = %q, want dhcpserveroptionsetaliases", got)
	}

	generation := providerForGeneration()
	assertRenamedField(t, generation.ResourcesMap["routeros_ipv6_nd_prefix"].Schema, "6to4_interface", "six_to_four_interface")
	assertRenamedField(t, generation.ResourcesMap["routeros_wifi_interworking"].Schema, "3gpp_info", "three_gpp_info")
	assertRenamedField(t, generation.ResourcesMap["routeros_wifi_interworking"].Schema, "3gpp_raw", "three_gpp_raw")

	runtime := routeros.Provider()
	if runtime.ResourcesMap["routeros_ipv6_nd_prefix"].Schema["6to4_interface"] == nil {
		t.Error("runtime provider lost the original 6to4_interface schema")
	}
	if runtime.ResourcesMap["routeros_wifi_interworking"].Schema["3gpp_info"] == nil || runtime.ResourcesMap["routeros_wifi_interworking"].Schema["3gpp_raw"] == nil {
		t.Error("runtime provider lost original 3gpp schemas")
	}
}

func TestNameIdentityResources(t *testing.T) {
	verdictData, err := os.ReadFile("name-uniqueness.json")
	if err != nil {
		t.Fatalf("cannot read pinned verdicts: %v", err)
	}
	var pinned struct {
		Verdicts []struct {
			Resource string `json:"resource"`
			Verdict  string `json:"verdict"`
		} `json:"verdicts"`
	}
	if err := json.Unmarshal(verdictData, &pinned); err != nil {
		t.Fatalf("cannot parse pinned verdicts: %v", err)
	}
	verdicts := map[string]string{}
	for _, v := range pinned.Verdicts {
		verdicts[v.Resource] = v.Verdict
	}
	for _, name := range nameIdentityResources {
		if got := verdicts[name]; got != "UNIQUE" {
			t.Errorf("%s has verdict %q in name-uniqueness.json, want UNIQUE — re-probe before flipping", name, got)
		}
	}
	for _, name := range nameIdentityResources {
		upstream := routeros.Provider().ResourcesMap[name]
		if upstream == nil {
			t.Fatalf("%s is not an upstream resource", name)
		}
		if got := upstream.Schema[routeros.MetaId].Default; got != int(routeros.Id) {
			t.Errorf("%s upstream ___id___ default = %v; the override is redundant, drop it from nameIdentityResources", name, got)
		}
	}
	runtime := withNameIdentity(routeros.Provider())
	for _, name := range nameIdentityResources {
		if got := runtime.ResourcesMap[name].Schema[routeros.MetaId].Default; got != int(routeros.Name) {
			t.Errorf("%s runtime ___id___ default = %v, want %v", name, got, int(routeros.Name))
		}
	}
	generation := providerForGeneration()
	for _, name := range nameIdentityResources {
		if got := generation.ResourcesMap[name].Schema[routeros.MetaId].Default; got != int(routeros.Id) {
			t.Errorf("generation schema for %s has ___id___ default = %v, want untouched upstream default", name, got)
		}
	}
}

func resourceNames[T any](resources map[string]T) map[string]struct{} {
	result := make(map[string]struct{}, len(resources))
	for name := range resources {
		result[name] = struct{}{}
	}
	return result
}

func assertRenamedField(t *testing.T, fields map[string]*schema.Schema, original, generated string) {
	t.Helper()
	if fields[original] != nil {
		t.Errorf("generation schema still contains invalid field %s", original)
	}
	if fields[generated] == nil {
		t.Errorf("generation schema does not contain %s", generated)
	}
	if !strings.Contains(fields[generated].Description, "TFTag="+original+",omitempty") {
		t.Errorf("generation field %s does not retain Terraform tag %s", generated, original)
	}
}
