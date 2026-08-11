package firewallfilter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1beta1 "github.com/sindrip/provider-routeros/apis/cluster/v1beta1"
	"github.com/sindrip/provider-routeros/rest"

	v1alpha1 "github.com/sindrip/provider-routeros/native/api/v1alpha1"
	"github.com/sindrip/provider-routeros/native/internal/connection"
)

type fakeMenu struct {
	plan    rest.Plan
	err     error
	spec    rest.MenuSpec
	desired []rest.Record
	calls   int
}

func (m *fakeMenu) Apply(_ context.Context, spec rest.MenuSpec, desired []rest.Record) (rest.Plan, error) {
	m.calls++
	m.spec = spec
	m.desired = desired
	return m.plan, m.err
}

type fakeConnector struct {
	menu  connection.Menu
	err   error
	name  string
	calls int
}

func (c *fakeConnector) Connect(_ context.Context, _ client.Reader, name string) (connection.Menu, error) {
	c.calls++
	c.name = name
	return c.menu, c.err
}

func TestReconcileAppliesOrderedMenuAndReportsIDs(t *testing.T) {
	scheme := testScheme(t)
	chain, action := "forward", "accept"
	object := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "main", UID: types.UID("uid-main"), Generation: 3},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedTolerate,
			Rows:              []v1alpha1.FirewallFilterRule{{Chain: &chain, Action: &action}},
		},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithStatusSubresource(&v1alpha1.FirewallFilterMenu{}).
		WithObjects(object).Build()
	remote := &fakeMenu{plan: rest.Plan{
		Steps:   []rest.Step{{Op: rest.OpCreate}},
		Matched: map[int]string{0: "*A"},
	}}
	connector := &fakeConnector{menu: remote}
	reconciler := &Reconciler{Client: kube, Connector: connector}
	request := ctrl.Request{NamespacedName: client.ObjectKey{Name: "main"}}

	// The first pass persists the finalizer before any remote mutation.
	if result, err := reconciler.Reconcile(context.Background(), request); err != nil || !result.Requeue {
		t.Fatalf("first Reconcile() = (%#v, %v), want immediate requeue", result, err)
	}
	if remote.calls != 0 {
		t.Fatalf("remote Apply called %d time(s) before finalizer persisted", remote.calls)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}

	if connector.name != "router" {
		t.Fatalf("connected ProviderConfig = %q, want router", connector.name)
	}
	if remote.spec.Path != menuPath || !remote.spec.Ordered || remote.spec.Unlisted != rest.UnlistedTolerate {
		t.Fatalf("REST menu spec = %#v", remote.spec)
	}
	wantDesired := []rest.Record{{"chain": "forward", "action": "accept"}}
	if !reflect.DeepEqual(remote.desired, wantDesired) {
		t.Fatalf("desired = %#v, want %#v", remote.desired, wantDesired)
	}

	got := &v1alpha1.FirewallFilterMenu{}
	if err := kube.Get(context.Background(), client.ObjectKey{Name: "main"}, got); err != nil {
		t.Fatal(err)
	}
	if got.Status.ObservedGeneration != 3 {
		t.Errorf("observedGeneration = %d, want 3", got.Status.ObservedGeneration)
	}
	if len(got.Status.Rows) != 1 || got.Status.Rows[0].ID != "*A" {
		t.Errorf("row status = %#v, want id *A", got.Status.Rows)
	}
	ready := condition(got, conditionReady)
	if ready == nil || ready.Status != metav1.ConditionTrue || ready.Reason != "Available" {
		t.Errorf("Ready condition = %#v", ready)
	}
	usage := &clusterv1beta1.ProviderConfigUsage{}
	if err := kube.Get(context.Background(), client.ObjectKey{Name: "uid-main"}, usage); err != nil {
		t.Fatalf("get ProviderConfigUsage: %v", err)
	}
	if usage.ProviderConfigReference.Name != "router" || usage.ResourceReference.Name != "main" {
		t.Errorf("ProviderConfigUsage = %#v", usage.ProviderConfigUsage)
	}
}

func TestReconcileFromProviderConfigThroughREST(t *testing.T) {
	var mu sync.Mutex
	var rows []rest.Record
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, request.Method+" "+request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/rest/ip/firewall/filter":
			if err := json.NewEncoder(response).Encode(rows); err != nil {
				t.Errorf("encode list: %v", err)
			}
		case request.Method == http.MethodPut && request.URL.Path == "/rest/ip/firewall/filter":
			row := rest.Record{}
			if err := json.NewDecoder(request.Body).Decode(&row); err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			row[rest.IDField] = "*A"
			rows = append(rows, row)
			if err := json.NewEncoder(response).Encode(row); err != nil {
				t.Errorf("encode created row: %v", err)
			}
		default:
			http.Error(response, "unexpected request", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	scheme := testScheme(t)
	chain, action := "forward", "accept"
	object := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "main", UID: types.UID("uid-main")},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedTolerate,
			Rows:              []v1alpha1.FirewallFilterRule{{Chain: &chain, Action: &action}},
		},
	}
	providerConfig := &clusterv1beta1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "router"},
		Spec: clusterv1beta1.ProviderConfigSpec{Credentials: clusterv1beta1.ProviderCredentials{
			Source: xpv2.CredentialsSourceSecret,
			CommonCredentialSelectors: xpv2.CommonCredentialSelectors{SecretRef: &xpv2.SecretKeySelector{
				SecretReference: xpv2.SecretReference{Name: "router", Namespace: "crossplane-system"},
				Key:             "credentials",
			}},
		}},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "router", Namespace: "crossplane-system"},
		StringData: map[string]string{
			"credentials": `{"hosturl":"` + server.URL + `","username":"admin","password":"secret"}`,
		},
	}
	// The fake client does not run the API server conversion that normally
	// copies StringData into Data.
	secret.Data = map[string][]byte{"credentials": []byte(secret.StringData["credentials"])}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithStatusSubresource(&v1alpha1.FirewallFilterMenu{}).
		WithObjects(object, providerConfig, secret).Build()
	reconciler := &Reconciler{Client: kube, Connector: &connection.ProviderConfigConnector{}}
	request := ctrl.Request{NamespacedName: client.ObjectKey{Name: "main"}}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("persist finalizer: %v", err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("apply through REST: %v", err)
	}

	mu.Lock()
	gotCalls := append([]string(nil), calls...)
	mu.Unlock()
	wantCalls := []string{
		"GET /rest/ip/firewall/filter",
		"PUT /rest/ip/firewall/filter",
		"GET /rest/ip/firewall/filter",
	}
	if !reflect.DeepEqual(gotCalls, wantCalls) {
		t.Fatalf("REST calls = %#v, want %#v", gotCalls, wantCalls)
	}
	got := &v1alpha1.FirewallFilterMenu{}
	if err := kube.Get(context.Background(), client.ObjectKey{Name: "main"}, got); err != nil {
		t.Fatal(err)
	}
	if len(got.Status.Rows) != 1 || got.Status.Rows[0].ID != "*A" {
		t.Fatalf("row status = %#v", got.Status.Rows)
	}
}

func TestReconcileSurfacesApplyFailure(t *testing.T) {
	scheme := testScheme(t)
	object := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "main", UID: types.UID("uid-main"), Finalizers: []string{finalizer}},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedPrune,
			Rows:              []v1alpha1.FirewallFilterRule{{}},
		},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithStatusSubresource(&v1alpha1.FirewallFilterMenu{}).
		WithObjects(object).Build()
	remote := &fakeMenu{err: errors.New("ambiguous row")}
	reconciler := &Reconciler{Client: kube, Connector: &fakeConnector{menu: remote}}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "main"}})
	if err == nil {
		t.Fatal("Reconcile() unexpectedly succeeded")
	}
	got := &v1alpha1.FirewallFilterMenu{}
	if getErr := kube.Get(context.Background(), client.ObjectKey{Name: "main"}, got); getErr != nil {
		t.Fatal(getErr)
	}
	ready := condition(got, conditionReady)
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != "ApplyError" {
		t.Fatalf("Ready condition = %#v", ready)
	}
}

func TestReconcileDeletePolicyDeletesEveryRemoteRow(t *testing.T) {
	scheme := testScheme(t)
	now := metav1.Now()
	object := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "main",
			Finalizers:        []string{finalizer},
			DeletionTimestamp: &now,
		},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedPrune,
			DeletionPolicy:    v1alpha1.DeletionDelete,
		},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithObjects(object).Build()
	remote := &fakeMenu{plan: rest.Plan{Matched: map[int]string{}}}
	reconciler := &Reconciler{Client: kube, Connector: &fakeConnector{menu: remote}}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "main"}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if remote.calls != 1 || remote.spec != deleteAllSpec || remote.desired != nil {
		t.Fatalf("delete Apply = calls %d, spec %#v, desired %#v", remote.calls, remote.spec, remote.desired)
	}
}

func TestReconcileDeletingNonOwnerNeverTouchesRouter(t *testing.T) {
	scheme := testScheme(t)
	older := metav1.Now()
	newer := metav1.NewTime(older.Add(1))
	first := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "first", UID: types.UID("uid-first"), CreationTimestamp: older, Finalizers: []string{finalizer}},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedPrune,
		},
	}
	now := metav1.Now()
	second := first.DeepCopy()
	second.Name = "second"
	second.UID = types.UID("uid-second")
	second.CreationTimestamp = newer
	second.DeletionTimestamp = &now
	second.Spec.DeletionPolicy = v1alpha1.DeletionDelete
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithObjects(first, second).Build()
	connector := &fakeConnector{menu: &fakeMenu{}}
	reconciler := &Reconciler{Client: kube, Connector: connector}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "second"}}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if connector.calls != 0 {
		t.Fatalf("connector called %d time(s) while deleting non-owner", connector.calls)
	}
}

func TestReconcileDoesNotFightAnExistingOwner(t *testing.T) {
	scheme := testScheme(t)
	older := metav1.Now()
	newer := metav1.NewTime(older.Add(1))
	first := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "first", UID: types.UID("uid-first"), CreationTimestamp: older, Finalizers: []string{finalizer}},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedTolerate,
			Rows:              []v1alpha1.FirewallFilterRule{},
		},
	}
	second := first.DeepCopy()
	second.Name = "second"
	second.UID = types.UID("uid-second")
	second.CreationTimestamp = newer
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithStatusSubresource(&v1alpha1.FirewallFilterMenu{}).
		WithObjects(first, second).Build()
	connector := &fakeConnector{menu: &fakeMenu{}}
	reconciler := &Reconciler{Client: kube, Connector: connector}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "second"}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("ownership conflict did not schedule another ownership check")
	}
	if connector.calls != 0 {
		t.Fatalf("connector called %d time(s) for non-owner", connector.calls)
	}
	got := &v1alpha1.FirewallFilterMenu{}
	if err := kube.Get(context.Background(), client.ObjectKey{Name: "second"}, got); err != nil {
		t.Fatal(err)
	}
	ready := condition(got, conditionReady)
	if ready == nil || ready.Reason != "OwnershipConflict" {
		t.Fatalf("Ready condition = %#v", ready)
	}
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := clusterv1beta1.SchemeBuilder.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func condition(object *v1alpha1.FirewallFilterMenu, kind string) *metav1.Condition {
	for i := range object.Status.Conditions {
		if object.Status.Conditions[i].Type == kind {
			return &object.Status.Conditions[i]
		}
	}
	return nil
}

func providerIndexValues(object client.Object) []string {
	menu := object.(*v1alpha1.FirewallFilterMenu)
	return []string{menu.Spec.ProviderConfigRef.Name}
}
