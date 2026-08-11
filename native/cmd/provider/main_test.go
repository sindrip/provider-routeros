package main

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	providerv1alpha1 "github.com/sindrip/provider-routeros/native/api/provider/v1alpha1"
)

func TestNativeSchemeOmitsClusterProviderConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := addProviderConfigTypes(scheme); err != nil {
		t.Fatal(err)
	}
	if _, err := scheme.New(providerv1alpha1.ProviderConfigGroupVersionKind); err != nil {
		t.Fatalf("namespaced ProviderConfig is not registered: %v", err)
	}
	if _, err := scheme.New(providerv1alpha1.SchemeGroupVersion.WithKind("ClusterProviderConfig")); err == nil {
		t.Fatal("ClusterProviderConfig is registered in the native provider scheme")
	}
}
