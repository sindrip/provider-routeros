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

	"github.com/sindrip/provider-routeros/rest"

	providerv1alpha1 "github.com/sindrip/provider-routeros/native/api/provider/v1alpha1"
	v1alpha1 "github.com/sindrip/provider-routeros/native/api/v1alpha1"
	"github.com/sindrip/provider-routeros/native/internal/connection"
)

type fakeMenu struct {
	plan              rest.Plan
	checkedPlan       *rest.Plan
	err               error
	planErr           error
	spec              rest.MenuSpec
	desired           []rest.Record
	calls             int
	planCalls         int
	applyCheckedCalls int
}

const testNamespace = "tenant-a"

func (m *fakeMenu) Plan(_ context.Context, spec rest.MenuSpec, desired []rest.Record) (rest.Plan, error) {
	m.planCalls++
	m.spec = spec
	m.desired = desired
	return m.plan, m.planErr
}

func (m *fakeMenu) Apply(_ context.Context, spec rest.MenuSpec, desired []rest.Record) (rest.Plan, error) {
	m.calls++
	m.spec = spec
	m.desired = desired
	return m.plan, m.err
}

func (m *fakeMenu) ApplyChecked(_ context.Context, spec rest.MenuSpec, desired []rest.Record, approve func(rest.Plan) error) (rest.Plan, error) {
	m.applyCheckedCalls++
	m.spec = spec
	m.desired = desired
	plan := m.plan
	if m.checkedPlan != nil {
		plan = *m.checkedPlan
	}
	if err := approve(plan); err != nil {
		return plan, err
	}
	if m.err != nil {
		return plan, m.err
	}
	m.calls++
	return plan, nil
}

type fakeConnector struct {
	menu        connection.Menu
	fingerprint string
	target      string
	targets     map[string]string
	err         error
	namespace   string
	name        string
	calls       int
}

func (c *fakeConnector) Connect(_ context.Context, _ client.Reader, namespace, name string) (connection.Connection, error) {
	c.calls++
	c.namespace = namespace
	c.name = name
	target := c.target
	if c.targets != nil {
		target = c.targets[providerKey(namespace, name)]
	}
	return connection.Connection{Menu: c.menu, Fingerprint: c.fingerprint, TargetFingerprint: target}, c.err
}

func TestReconcileAppliesOrderedMenuAndReportsIDs(t *testing.T) {
	scheme := testScheme(t)
	chain, action := "forward", "accept"
	object := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: testNamespace, UID: types.UID("uid-main"), Generation: 3},
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
	request := ctrl.Request{NamespacedName: testKey("main")}

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

	if connector.namespace != testNamespace || connector.name != "router" {
		t.Fatalf("connected ProviderConfig = %s/%s, want %s/router", connector.namespace, connector.name, testNamespace)
	}
	if remote.spec.Path != menuPath || !remote.spec.Ordered || remote.spec.Unlisted != rest.UnlistedTolerate {
		t.Fatalf("REST menu spec = %#v", remote.spec)
	}
	if !reflect.DeepEqual(remote.spec.Ignore, dynamicIgnore) {
		t.Fatalf("ignored rows = %#v, want RouterOS dynamic selectors", remote.spec.Ignore)
	}
	wantDesired := []rest.Record{{"chain": "forward", "action": "accept"}}
	if !reflect.DeepEqual(remote.desired, wantDesired) {
		t.Fatalf("desired = %#v, want %#v", remote.desired, wantDesired)
	}

	got := &v1alpha1.FirewallFilterMenu{}
	if err := kube.Get(context.Background(), testKey("main"), got); err != nil {
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
	usage := &providerv1alpha1.ProviderConfigUsage{}
	if err := kube.Get(context.Background(), testKey("uid-main"), usage); err != nil {
		t.Fatalf("get ProviderConfigUsage: %v", err)
	}
	if usage.ProviderConfigReference.Kind != providerv1alpha1.ProviderConfigKind || usage.ProviderConfigReference.Name != "router" || usage.ResourceReference.Name != "main" {
		t.Errorf("ProviderConfigUsage = %#v", usage.TypedProviderConfigUsage)
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
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: testNamespace, UID: types.UID("uid-main")},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedTolerate,
			Rows:              []v1alpha1.FirewallFilterRule{{Chain: &chain, Action: &action}},
		},
	}
	providerConfig := &providerv1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "router", Namespace: testNamespace},
		Spec: providerv1alpha1.ProviderConfigSpec{
			Endpoint: server.URL,
			Credentials: providerv1alpha1.ProviderCredentials{SecretRef: providerv1alpha1.ProviderCredentialSecretReference{
				Name: "router",
			}},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "router", Namespace: testNamespace},
		StringData: map[string]string{"username": "admin", "password": "secret"},
	}
	// The fake client does not run the API server conversion that normally
	// copies StringData into Data.
	secret.Data = map[string][]byte{"username": []byte(secret.StringData["username"]), "password": []byte(secret.StringData["password"])}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithStatusSubresource(&v1alpha1.FirewallFilterMenu{}).
		WithObjects(object, providerConfig, secret).Build()
	reconciler := &Reconciler{Client: kube, Connector: &connection.ProviderConfigConnector{}}
	request := ctrl.Request{NamespacedName: testKey("main")}

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
	if err := kube.Get(context.Background(), testKey("main"), got); err != nil {
		t.Fatal(err)
	}
	if len(got.Status.Rows) != 1 || got.Status.Rows[0].ID != "*A" {
		t.Fatalf("row status = %#v", got.Status.Rows)
	}
}

func TestReconcileSurfacesApplyFailure(t *testing.T) {
	scheme := testScheme(t)
	object := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: testNamespace, UID: types.UID("uid-main"), Finalizers: []string{finalizer}},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedTolerate,
			Rows:              []v1alpha1.FirewallFilterRule{{}},
		},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithStatusSubresource(&v1alpha1.FirewallFilterMenu{}).
		WithObjects(object).Build()
	remote := &fakeMenu{err: errors.New("ambiguous row")}
	reconciler := &Reconciler{Client: kube, Connector: &fakeConnector{menu: remote}}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: testKey("main")})
	if err == nil {
		t.Fatal("Reconcile() unexpectedly succeeded")
	}
	got := &v1alpha1.FirewallFilterMenu{}
	if getErr := kube.Get(context.Background(), testKey("main"), got); getErr != nil {
		t.Fatal(getErr)
	}
	ready := condition(got, conditionReady)
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != "ApplyError" {
		t.Fatalf("Ready condition = %#v", ready)
	}
}

func TestFirstPruneRequiresMatchingApproval(t *testing.T) {
	scheme := testScheme(t)
	object := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: testNamespace, UID: types.UID("uid-main"), Finalizers: []string{finalizer}},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedPrune,
			Rows:              []v1alpha1.FirewallFilterRule{},
		},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithStatusSubresource(&v1alpha1.FirewallFilterMenu{}).
		WithObjects(object).Build()
	remote := &fakeMenu{plan: rest.Plan{Steps: []rest.Step{{
		Op: rest.OpDelete, ID: "*1", Row: rest.Record{rest.IDField: "*1", "comment": "existing"},
	}}}}
	reconciler := &Reconciler{Client: kube, Connector: &fakeConnector{menu: remote, fingerprint: "connection-v1"}}
	request := ctrl.Request{NamespacedName: testKey("main")}

	result, err := reconciler.Reconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview Reconcile() error = %v", err)
	}
	if result.RequeueAfter == 0 || remote.planCalls != 1 || remote.calls != 0 || remote.applyCheckedCalls != 0 {
		t.Fatalf("preview result=%#v plan=%d apply=%d checked=%d", result, remote.planCalls, remote.calls, remote.applyCheckedCalls)
	}
	got := &v1alpha1.FirewallFilterMenu{}
	if err := kube.Get(context.Background(), testKey("main"), got); err != nil {
		t.Fatal(err)
	}
	if got.Status.PendingPlan == nil || got.Status.PendingPlan.Deletes != 1 || len(got.Status.PendingPlan.ApprovalToken) != 64 {
		t.Fatalf("pending plan = %#v", got.Status.PendingPlan)
	}
	if len(got.Status.PendingPlan.DeleteRows) != 1 || got.Status.PendingPlan.DeleteRows[0].Comment != "existing" {
		t.Fatalf("delete preview = %#v", got.Status.PendingPlan.DeleteRows)
	}
	ready := condition(got, conditionReady)
	if ready == nil || ready.Reason != "AdoptionPending" || ready.Status != metav1.ConditionFalse {
		t.Fatalf("Ready condition = %#v", ready)
	}

	before := got.DeepCopy()
	got.Annotations = map[string]string{PruneApprovalAnnotation: got.Status.PendingPlan.ApprovalToken}
	if err := kube.Patch(context.Background(), got, client.MergeFrom(before)); err != nil {
		t.Fatalf("approve pending plan: %v", err)
	}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("approved Reconcile() error = %v", err)
	}
	if remote.planCalls != 2 || remote.applyCheckedCalls != 1 || remote.calls != 1 {
		t.Fatalf("approved plan=%d checked=%d apply=%d", remote.planCalls, remote.applyCheckedCalls, remote.calls)
	}
	if err := kube.Get(context.Background(), testKey("main"), got); err != nil {
		t.Fatal(err)
	}
	if !got.Status.Adopted || got.Status.AdoptedConnection != "connection-v1" || got.Status.PendingPlan != nil {
		t.Fatalf("adoption status = %#v", got.Status)
	}
}

func TestChangedPlanInvalidatesPruneApproval(t *testing.T) {
	scheme := testScheme(t)
	object := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: testNamespace, UID: types.UID("uid-main"), Finalizers: []string{finalizer}},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedPrune,
			Rows:              []v1alpha1.FirewallFilterRule{},
		},
	}
	initial := rest.Plan{Steps: []rest.Step{{Op: rest.OpDelete, ID: "*1", Row: rest.Record{rest.IDField: "*1"}}}}
	changed := rest.Plan{Steps: []rest.Step{{Op: rest.OpDelete, ID: "*2", Row: rest.Record{rest.IDField: "*2"}}}}
	remote := &fakeMenu{plan: initial, checkedPlan: &changed}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithStatusSubresource(&v1alpha1.FirewallFilterMenu{}).
		WithObjects(object).Build()
	reconciler := &Reconciler{Client: kube, Connector: &fakeConnector{menu: remote, fingerprint: "connection-v1"}}
	request := ctrl.Request{NamespacedName: testKey("main")}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	got := &v1alpha1.FirewallFilterMenu{}
	if err := kube.Get(context.Background(), testKey("main"), got); err != nil {
		t.Fatal(err)
	}
	oldToken := got.Status.PendingPlan.ApprovalToken
	before := got.DeepCopy()
	got.Annotations = map[string]string{PruneApprovalAnnotation: oldToken}
	if err := kube.Patch(context.Background(), got, client.MergeFrom(before)); err != nil {
		t.Fatal(err)
	}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("changed-plan Reconcile() error = %v", err)
	}
	if remote.calls != 0 {
		t.Fatalf("changed approved plan performed %d mutation(s)", remote.calls)
	}
	if err := kube.Get(context.Background(), testKey("main"), got); err != nil {
		t.Fatal(err)
	}
	if got.Status.PendingPlan == nil || got.Status.PendingPlan.ApprovalToken == oldToken {
		t.Fatalf("stale token was not replaced: %#v", got.Status.PendingPlan)
	}
}

func TestNonDestructiveFirstPruneAdoptsAutomatically(t *testing.T) {
	scheme := testScheme(t)
	object := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: testNamespace, UID: types.UID("uid-main"), Finalizers: []string{finalizer}},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedPrune,
			Rows:              []v1alpha1.FirewallFilterRule{},
		},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithStatusSubresource(&v1alpha1.FirewallFilterMenu{}).
		WithObjects(object).Build()
	remote := &fakeMenu{plan: rest.Plan{Matched: map[int]string{}}}
	reconciler := &Reconciler{Client: kube, Connector: &fakeConnector{menu: remote, fingerprint: "connection-v1"}}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: testKey("main")}); err != nil {
		t.Fatal(err)
	}
	if remote.planCalls != 1 || remote.calls != 1 || remote.applyCheckedCalls != 1 {
		t.Fatalf("plan=%d apply=%d checked=%d", remote.planCalls, remote.calls, remote.applyCheckedCalls)
	}
	got := &v1alpha1.FirewallFilterMenu{}
	if err := kube.Get(context.Background(), testKey("main"), got); err != nil {
		t.Fatal(err)
	}
	if !got.Status.Adopted || got.Status.AdoptedConnection != "connection-v1" {
		t.Fatalf("adoption status = %#v", got.Status)
	}
}

func TestChangedConnectionRequiresFreshAdoption(t *testing.T) {
	scheme := testScheme(t)
	object := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: testNamespace, UID: types.UID("uid-main"), Finalizers: []string{finalizer}},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedPrune,
			Rows:              []v1alpha1.FirewallFilterRule{},
		},
		Status: v1alpha1.FirewallFilterMenuStatus{Adopted: true, AdoptedConnection: "old-connection"},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithStatusSubresource(&v1alpha1.FirewallFilterMenu{}).
		WithObjects(object).Build()
	remote := &fakeMenu{plan: rest.Plan{Steps: []rest.Step{{
		Op: rest.OpDelete, ID: "*1", Row: rest.Record{rest.IDField: "*1", "comment": "existing"},
	}}}}
	reconciler := &Reconciler{Client: kube, Connector: &fakeConnector{menu: remote, fingerprint: "new-connection"}}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: testKey("main")}); err != nil {
		t.Fatal(err)
	}
	if remote.calls != 0 || remote.applyCheckedCalls != 0 {
		t.Fatal("changed connection pruned before fresh approval")
	}
	got := &v1alpha1.FirewallFilterMenu{}
	if err := kube.Get(context.Background(), testKey("main"), got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Adopted || got.Status.PendingPlan == nil {
		t.Fatalf("connection change did not reset adoption: %#v", got.Status)
	}
}

func TestPlanTokenIgnoresCountersButNotRuleChanges(t *testing.T) {
	base := rest.Plan{Steps: []rest.Step{{
		Op: rest.OpDelete, ID: "*1", Row: rest.Record{
			rest.IDField: "*1", "chain": "input", "action": "drop", "bytes": "1", "packets": "1",
		},
	}}}
	countersChanged := rest.Plan{Steps: []rest.Step{{
		Op: rest.OpDelete, ID: "*1", Row: rest.Record{
			rest.IDField: "*1", "chain": "input", "action": "drop", "bytes": "9001", "packets": "42",
		},
	}}}
	ruleChanged := rest.Plan{Steps: []rest.Step{{
		Op: rest.OpDelete, ID: "*1", Row: rest.Record{
			rest.IDField: "*1", "chain": "input", "action": "accept", "bytes": "9001", "packets": "42",
		},
	}}}

	baseToken := planToken("connection", base)
	if got := planToken("connection", countersChanged); got != baseToken {
		t.Fatalf("counter change altered approval token: %s != %s", got, baseToken)
	}
	if got := planToken("connection", ruleChanged); got == baseToken {
		t.Fatal("firewall rule change did not alter approval token")
	}
	if got := planToken("different-connection", base); got == baseToken {
		t.Fatal("connection change did not alter approval token")
	}
}

func TestReconcileDeletePolicyDeletesEveryRemoteRow(t *testing.T) {
	scheme := testScheme(t)
	now := metav1.Now()
	object := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "main",
			Namespace:         testNamespace,
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

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: testKey("main")}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if remote.calls != 1 || !reflect.DeepEqual(remote.spec, deleteAllStaticSpec) || remote.desired != nil {
		t.Fatalf("delete Apply = calls %d, spec %#v, desired %#v", remote.calls, remote.spec, remote.desired)
	}
}

func TestReconcileDeletingNonOwnerNeverTouchesRouter(t *testing.T) {
	scheme := testScheme(t)
	older := metav1.Now()
	newer := metav1.NewTime(older.Add(1))
	first := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "first", Namespace: testNamespace, UID: types.UID("uid-first"), CreationTimestamp: older, Finalizers: []string{finalizer}},
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
	remote := &fakeMenu{}
	connector := &fakeConnector{menu: remote}
	reconciler := &Reconciler{Client: kube, Connector: connector}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: testKey("second")}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if remote.calls != 0 {
		t.Fatalf("remote Apply called %d time(s) while deleting non-owner", remote.calls)
	}
}

func TestReconcileDoesNotFightAnExistingOwner(t *testing.T) {
	scheme := testScheme(t)
	older := metav1.Now()
	newer := metav1.NewTime(older.Add(1))
	first := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "first", Namespace: testNamespace, UID: types.UID("uid-first"), CreationTimestamp: older, Finalizers: []string{finalizer}},
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
	remote := &fakeMenu{}
	connector := &fakeConnector{menu: remote}
	reconciler := &Reconciler{Client: kube, Connector: connector}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: testKey("second")})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("ownership conflict did not schedule another ownership check")
	}
	if remote.calls != 0 {
		t.Fatalf("remote Apply called %d time(s) for non-owner", remote.calls)
	}
	got := &v1alpha1.FirewallFilterMenu{}
	if err := kube.Get(context.Background(), testKey("second"), got); err != nil {
		t.Fatal(err)
	}
	ready := condition(got, conditionReady)
	if ready == nil || ready.Reason != "OwnershipConflict" {
		t.Fatalf("Ready condition = %#v", ready)
	}
}

func TestReconcilePreventsCrossNamespaceOwnershipOfSameRouter(t *testing.T) {
	scheme := testScheme(t)
	older := metav1.Now()
	newer := metav1.NewTime(older.Add(1))
	first := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "first", Namespace: "tenant-a", UID: types.UID("uid-first"), CreationTimestamp: older, Finalizers: []string{finalizer}},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedTolerate,
		},
	}
	second := first.DeepCopy()
	second.Name = "second"
	second.Namespace = "tenant-b"
	second.UID = types.UID("uid-second")
	second.CreationTimestamp = newer
	remote := &fakeMenu{}
	connector := &fakeConnector{menu: remote, target: "same-router"}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithStatusSubresource(&v1alpha1.FirewallFilterMenu{}).
		WithObjects(first, second).Build()
	reconciler := &Reconciler{Client: kube, Connector: connector}

	request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(second)}
	result, err := reconciler.Reconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter == 0 || remote.calls != 0 {
		t.Fatalf("cross-namespace conflict result=%#v remote calls=%d", result, remote.calls)
	}
	got := &v1alpha1.FirewallFilterMenu{}
	if err := kube.Get(context.Background(), request.NamespacedName, got); err != nil {
		t.Fatal(err)
	}
	ready := condition(got, conditionReady)
	if ready == nil || ready.Reason != "OwnershipConflict" {
		t.Fatalf("Ready condition = %#v", ready)
	}
}

func TestReconcileAllowsSameProviderConfigNameForDifferentRouters(t *testing.T) {
	scheme := testScheme(t)
	older := metav1.Now()
	newer := metav1.NewTime(older.Add(1))
	first := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "first", Namespace: "tenant-a", UID: types.UID("uid-first"), CreationTimestamp: older, Finalizers: []string{finalizer}},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
			Unlisted:          v1alpha1.UnlistedTolerate,
		},
	}
	second := first.DeepCopy()
	second.Name = "second"
	second.Namespace = "tenant-b"
	second.UID = types.UID("uid-second")
	second.CreationTimestamp = newer
	remote := &fakeMenu{}
	connector := &fakeConnector{
		menu: remote,
		targets: map[string]string{
			"tenant-a/router": "router-a",
			"tenant-b/router": "router-b",
		},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithStatusSubresource(&v1alpha1.FirewallFilterMenu{}).
		WithObjects(first, second).Build()
	reconciler := &Reconciler{Client: kube, Connector: connector}

	request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(second)}
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if remote.calls != 1 {
		t.Fatalf("remote Apply called %d time(s), want 1", remote.calls)
	}
}

func TestProviderConfigAndSecretEventsEnqueueOnlyReferencingMenus(t *testing.T) {
	scheme := testScheme(t)
	menu := &v1alpha1.FirewallFilterMenu{
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: "tenant-a"},
		Spec: v1alpha1.FirewallFilterMenuSpec{
			ProviderConfigRef: v1alpha1.ProviderConfigReference{Name: "router"},
		},
	}
	otherMenu := menu.DeepCopy()
	otherMenu.Name = "other"
	otherMenu.Spec.ProviderConfigRef.Name = "other-router"
	crossNamespace := menu.DeepCopy()
	crossNamespace.Name = "cross-namespace"
	crossNamespace.Namespace = "tenant-b"
	config := &providerv1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "router", Namespace: "tenant-a"},
		Spec: providerv1alpha1.ProviderConfigSpec{
			Endpoint: "https://router.example",
			Credentials: providerv1alpha1.ProviderCredentials{SecretRef: providerv1alpha1.ProviderCredentialSecretReference{
				Name: "router-creds",
			}},
			TLS: &providerv1alpha1.ProviderTLSConfig{CASecretRef: &xpv2.LocalSecretKeySelector{Name: "router-ca", Key: "ca.crt"}},
		},
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1alpha1.FirewallFilterMenu{}, providerIndex, providerIndexValues).
		WithIndex(&providerv1alpha1.ProviderConfig{}, secretIndex, providerSecretReferenceKeys).
		WithObjects(menu, otherMenu, crossNamespace, config).Build()
	reconciler := &Reconciler{Client: kube}

	want := []ctrl.Request{{NamespacedName: client.ObjectKeyFromObject(menu)}}
	if got := reconciler.requestsForProviderConfig(context.Background(), config); !reflect.DeepEqual(got, want) {
		t.Fatalf("ProviderConfig requests = %#v, want %#v", got, want)
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "router-creds", Namespace: "tenant-a"}}
	if got := reconciler.requestsForSecret(context.Background(), secret); !reflect.DeepEqual(got, want) {
		t.Fatalf("credentials Secret requests = %#v, want %#v", got, want)
	}
	ca := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "router-ca", Namespace: "tenant-a"}}
	if got := reconciler.requestsForSecret(context.Background(), ca); !reflect.DeepEqual(got, want) {
		t.Fatalf("CA Secret requests = %#v, want %#v", got, want)
	}
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := providerv1alpha1.AddToScheme(scheme); err != nil {
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
	return []string{providerKey(menu.Namespace, menu.Spec.ProviderConfigRef.Name)}
}

func testKey(name string) client.ObjectKey {
	return client.ObjectKey{Namespace: testNamespace, Name: name}
}
