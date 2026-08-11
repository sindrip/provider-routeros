package v1alpha1

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
)

// GetCondition returns the requested ProviderConfig condition.
func (p *ProviderConfig) GetCondition(conditionType xpv2.ConditionType) xpv2.Condition {
	return p.Status.GetCondition(conditionType)
}

// GetUsers returns the number of active ProviderConfigUsage objects.
func (p *ProviderConfig) GetUsers() int64 {
	return p.Status.Users
}

// SetConditions updates ProviderConfig conditions.
func (p *ProviderConfig) SetConditions(conditions ...xpv2.Condition) {
	p.Status.SetConditions(conditions...)
}

// SetUsers updates the number of active ProviderConfigUsage objects.
func (p *ProviderConfig) SetUsers(users int64) {
	p.Status.Users = users
}

// GetProviderConfigReference returns the referenced ProviderConfig.
func (p *ProviderConfigUsage) GetProviderConfigReference() xpv2.ProviderConfigReference {
	return p.ProviderConfigReference
}

// SetProviderConfigReference updates the referenced ProviderConfig.
func (p *ProviderConfigUsage) SetProviderConfigReference(reference xpv2.ProviderConfigReference) {
	p.ProviderConfigReference = reference
}

// GetResourceReference returns the resource using the ProviderConfig.
func (p *ProviderConfigUsage) GetResourceReference() xpv2.TypedReference {
	return p.ResourceReference
}

// SetResourceReference updates the resource using the ProviderConfig.
func (p *ProviderConfigUsage) SetResourceReference(reference xpv2.TypedReference) {
	p.ResourceReference = reference
}

// GetItems returns ProviderConfigUsage items through Crossplane's interface.
func (p *ProviderConfigUsageList) GetItems() []resource.ProviderConfigUsage {
	items := make([]resource.ProviderConfigUsage, len(p.Items))
	for i := range p.Items {
		items[i] = &p.Items[i]
	}
	return items
}

var (
	_ resource.ProviderConfig           = &ProviderConfig{}
	_ resource.TypedProviderConfigUsage = &ProviderConfigUsage{}
	_ resource.ProviderConfigUsageList  = &ProviderConfigUsageList{}
)
