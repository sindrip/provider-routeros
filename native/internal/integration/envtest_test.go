package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	v1alpha1 "github.com/sindrip/provider-routeros/native/api/v1alpha1"
)

func TestFirewallFilterMenuCRD(t *testing.T) {
	assets := os.Getenv("KUBEBUILDER_ASSETS")
	if assets == "" {
		t.Skip("set KUBEBUILDER_ASSETS to run envtest admission coverage")
	}
	crds, err := filepath.Abs("../../config/crd")
	if err != nil {
		t.Fatal(err)
	}
	environment := &envtest.Environment{
		BinaryAssetsDirectory: assets,
		CRDDirectoryPaths:     []string{crds},
	}
	config, err := environment.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := environment.Stop(); err != nil {
			t.Errorf("stop envtest: %v", err)
		}
	})

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	invalid := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "invalid"},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedPolicy("Guess"),
			Rows:              []v1alpha1.FirewallFilterRule{},
		},
	}
	if err := kube.Create(context.Background(), invalid); err == nil {
		t.Fatal("API server accepted an invalid unlisted policy")
	}
	invalidPolicyPair := invalid.DeepCopy()
	invalidPolicyPair.Name = "invalid-policy-pair"
	invalidPolicyPair.Spec.Unlisted = v1alpha1.UnlistedTolerate
	invalidPolicyPair.Spec.DeletionPolicy = v1alpha1.DeletionDelete
	if err := kube.Create(context.Background(), invalidPolicyPair); err == nil {
		t.Fatal("API server accepted deletionPolicy Delete without full-menu ownership")
	}

	valid := invalid.DeepCopy()
	valid.Name = "valid"
	valid.ResourceVersion = ""
	valid.Spec.Unlisted = v1alpha1.UnlistedTolerate
	if err := kube.Create(context.Background(), valid); err != nil {
		t.Fatalf("API server rejected a valid menu: %v", err)
	}
	if valid.Spec.DeletionPolicy != v1alpha1.DeletionOrphan {
		t.Fatalf("deletionPolicy default = %q, want %q", valid.Spec.DeletionPolicy, v1alpha1.DeletionOrphan)
	}
	valid.Status.PendingPlan = &v1alpha1.FirewallFilterPlanStatus{
		ApprovalToken: "0123456789abcdef",
		Deletes:       1,
		DeleteRows: []v1alpha1.FirewallFilterDeletePreview{{
			ID: "*1", Chain: "input", Action: "drop", Comment: "existing",
		}},
	}
	if err := kube.Status().Update(context.Background(), valid); err != nil {
		t.Fatalf("API server rejected pending-plan status: %v", err)
	}
	observed := &v1alpha1.FirewallFilterMenu{}
	if err := kube.Get(context.Background(), client.ObjectKey{Name: "valid"}, observed); err != nil {
		t.Fatal(err)
	}
	if observed.Status.PendingPlan == nil || len(observed.Status.PendingPlan.DeleteRows) != 1 {
		t.Fatalf("pending-plan status did not round-trip: %#v", observed.Status.PendingPlan)
	}
}
