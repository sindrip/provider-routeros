package providerconfig

import (
	runtimeproviderconfig "github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/providerconfig"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	ctrl "sigs.k8s.io/controller-runtime"

	providerv1alpha1 "github.com/sindrip/provider-routeros/native/api/provider/v1alpha1"
)

// SetupWithManager accounts for namespaced ProviderConfigUsage objects and
// prevents a ProviderConfig from disappearing while it is still referenced.
func SetupWithManager(mgr ctrl.Manager) error {
	name := runtimeproviderconfig.ControllerName(providerv1alpha1.ProviderConfigGroupKind)
	kinds := resource.ProviderConfigKinds{
		Config:    providerv1alpha1.ProviderConfigGroupVersionKind,
		Usage:     providerv1alpha1.ProviderConfigUsageGroupVersionKind,
		UsageList: providerv1alpha1.ProviderConfigUsageListGroupVersionKind,
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&providerv1alpha1.ProviderConfig{}).
		Watches(&providerv1alpha1.ProviderConfigUsage{}, &resource.EnqueueRequestForProviderConfig{}).
		Complete(runtimeproviderconfig.NewReconciler(mgr, kinds))
}
