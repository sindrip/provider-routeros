package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	providerv1alpha1 "github.com/sindrip/provider-routeros/native/api/provider/v1alpha1"
	v1alpha1 "github.com/sindrip/provider-routeros/native/api/v1alpha1"
	providerconfigcontroller "github.com/sindrip/provider-routeros/native/internal/controller/providerconfig"
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
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := providerv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:  scheme,
		Metrics: server.Options{BindAddress: "0"},
	})
	if err != nil {
		t.Fatalf("create test manager: %v", err)
	}
	if err := providerconfigcontroller.SetupWithManager(mgr); err != nil {
		t.Fatalf("register ProviderConfig controller: %v", err)
	}
	managerContext, cancelManager := context.WithCancel(context.Background())
	managerDone := make(chan error, 1)
	go func() {
		managerDone <- mgr.Start(managerContext)
	}()
	if !mgr.GetCache().WaitForCacheSync(managerContext) {
		cancelManager()
		t.Fatal("ProviderConfig controller cache did not sync")
	}
	t.Cleanup(func() {
		cancelManager()
		select {
		case err := <-managerDone:
			if err != nil {
				t.Errorf("stop test manager: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("test manager did not stop")
		}
	})
	const namespace = "tenant-a"
	if err := kube.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}); err != nil {
		t.Fatalf("create test namespace: %v", err)
	}
	providerConfig := &providerv1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "router", Namespace: namespace},
		Spec: providerv1alpha1.ProviderConfigSpec{
			Endpoint: "https://router.example",
			Credentials: providerv1alpha1.ProviderCredentials{SecretRef: providerv1alpha1.ProviderCredentialSecretReference{
				Name: "router",
			}},
		},
	}
	if err := kube.Create(context.Background(), providerConfig); err != nil {
		t.Fatalf("API server rejected a namespaced ProviderConfig: %v", err)
	}
	if providerConfig.Spec.Credentials.SecretRef.UsernameKey != "username" || providerConfig.Spec.Credentials.SecretRef.PasswordKey != "password" {
		t.Fatalf("credential key defaults = %q/%q, want username/password",
			providerConfig.Spec.Credentials.SecretRef.UsernameKey,
			providerConfig.Spec.Credentials.SecretRef.PasswordKey,
		)
	}

	invalidProviderConfigs := map[string]func(*providerv1alpha1.ProviderConfig){
		"endpoint-without-scheme": func(config *providerv1alpha1.ProviderConfig) {
			config.Spec.Endpoint = "router.example"
		},
		"TLS-on-HTTP": func(config *providerv1alpha1.ProviderConfig) {
			config.Spec.Endpoint = "http://router.example"
			config.Spec.TLS = &providerv1alpha1.ProviderTLSConfig{}
		},
		"conflicting-TLS-settings": func(config *providerv1alpha1.ProviderConfig) {
			config.Spec.TLS = &providerv1alpha1.ProviderTLSConfig{
				InsecureSkipVerify: true,
				CASecretRef:        &xpv2.LocalSecretKeySelector{Name: "router-ca", Key: "ca.crt"},
			}
		},
		"short-request-timeout": func(config *providerv1alpha1.ProviderConfig) {
			config.Spec.RequestTimeout = &metav1.Duration{Duration: 4 * time.Second}
		},
	}
	for name, mutate := range invalidProviderConfigs {
		t.Run(name, func(t *testing.T) {
			config := providerConfig.DeepCopy()
			config.Name = name
			config.ResourceVersion = ""
			config.UID = ""
			config.Finalizers = nil
			config.Status = providerv1alpha1.ProviderConfigStatus{}
			mutate(config)
			if err := kube.Create(context.Background(), config); err == nil {
				t.Fatal("API server accepted an invalid ProviderConfig")
			}
		})
	}
	waitFor(t, "ProviderConfig finalizer", func() bool {
		observed := &providerv1alpha1.ProviderConfig{}
		if err := kube.Get(context.Background(), client.ObjectKeyFromObject(providerConfig), observed); err != nil {
			return false
		}
		return len(observed.Finalizers) > 0 && observed.Status.Users == 0
	})
	owner := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "usage-owner", Namespace: namespace}}
	if err := kube.Create(context.Background(), owner); err != nil {
		t.Fatalf("create usage owner: %v", err)
	}
	usage := &providerv1alpha1.ProviderConfigUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "usage",
			Namespace: namespace,
			Labels: map[string]string{
				xpv2.LabelKeyProviderName: providerConfig.Name,
				xpv2.LabelKeyProviderKind: providerv1alpha1.ProviderConfigKind,
			},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(owner, corev1.SchemeGroupVersion.WithKind("ConfigMap"))},
		},
		TypedProviderConfigUsage: xpv2.TypedProviderConfigUsage{
			ProviderConfigReference: xpv2.ProviderConfigReference{Kind: providerv1alpha1.ProviderConfigKind, Name: providerConfig.Name},
			ResourceReference:       xpv2.TypedReference{APIVersion: "v1", Kind: "ConfigMap", Name: owner.Name, UID: owner.UID},
		},
	}
	if err := kube.Create(context.Background(), usage); err != nil {
		t.Fatalf("create ProviderConfigUsage: %v", err)
	}
	waitFor(t, "ProviderConfig usage count", func() bool {
		observed := &providerv1alpha1.ProviderConfig{}
		if err := kube.Get(context.Background(), client.ObjectKeyFromObject(providerConfig), observed); err != nil {
			return false
		}
		return observed.Status.Users == 1
	})
	clusterConfigCRD := &apiextensionsv1.CustomResourceDefinition{}
	if err := kube.Get(context.Background(), client.ObjectKey{Name: "clusterproviderconfigs.routeros.m.sindrip.io"}, clusterConfigCRD); !apierrors.IsNotFound(err) {
		t.Fatalf("ClusterProviderConfig CRD unexpectedly installed: %v", err)
	}
	providerConfigCRD := &apiextensionsv1.CustomResourceDefinition{}
	if err := kube.Get(context.Background(), client.ObjectKey{Name: "providerconfigs.routeros.m.sindrip.io"}, providerConfigCRD); err != nil {
		t.Fatalf("get ProviderConfig CRD: %v", err)
	}
	if len(providerConfigCRD.Spec.Versions) != 1 || providerConfigCRD.Spec.Versions[0].Name != "v1alpha1" {
		t.Fatalf("ProviderConfig CRD versions = %#v, want only v1alpha1", providerConfigCRD.Spec.Versions)
	}

	invalid := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "invalid", Namespace: namespace},
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
	if err := kube.Get(context.Background(), client.ObjectKey{Name: "valid", Namespace: namespace}, observed); err != nil {
		t.Fatal(err)
	}
	if observed.Status.PendingPlan == nil || len(observed.Status.PendingPlan.DeleteRows) != 1 {
		t.Fatalf("pending-plan status did not round-trip: %#v", observed.Status.PendingPlan)
	}

	if err := kube.Delete(context.Background(), providerConfig); err != nil {
		t.Fatalf("delete in-use ProviderConfig: %v", err)
	}
	waitFor(t, "in-use ProviderConfig deletion protection", func() bool {
		observed := &providerv1alpha1.ProviderConfig{}
		if err := kube.Get(context.Background(), client.ObjectKeyFromObject(providerConfig), observed); err != nil {
			return false
		}
		return !observed.DeletionTimestamp.IsZero() && observed.Status.Users == 1
	})
	if err := kube.Delete(context.Background(), usage); err != nil {
		t.Fatalf("delete ProviderConfigUsage: %v", err)
	}
	waitFor(t, "unused ProviderConfig deletion", func() bool {
		err := kube.Get(context.Background(), client.ObjectKeyFromObject(providerConfig), &providerv1alpha1.ProviderConfig{})
		return apierrors.IsNotFound(err)
	})
}

func waitFor(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}
