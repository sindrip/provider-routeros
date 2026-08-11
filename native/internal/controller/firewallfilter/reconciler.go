package firewallfilter

import (
	"context"
	"errors"
	"fmt"
	"time"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1beta1 "github.com/sindrip/provider-routeros/apis/cluster/v1beta1"
	"github.com/sindrip/provider-routeros/rest"

	v1alpha1 "github.com/sindrip/provider-routeros/native/api/v1alpha1"
	"github.com/sindrip/provider-routeros/native/internal/connection"
)

const (
	menuPath        = "/ip/firewall/filter"
	finalizer       = "firewallfiltermenus.ip.routeros.sindrip.io/finalizer"
	defaultInterval = time.Minute
	conditionReady  = "Ready"
	providerIndex   = "spec.providerConfigRef.name"
)

var deleteAllSpec = rest.MenuSpec{
	Path:     menuPath,
	Ordered:  true,
	Unlisted: rest.UnlistedPrune,
}

// Connector resolves a ProviderConfig into a menu client.
type Connector interface {
	Connect(context.Context, client.Reader, string) (connection.Menu, error)
}

// Reconciler converges FirewallFilterMenu objects to RouterOS.
type Reconciler struct {
	client.Client
	Connector    Connector
	RequeueAfter time.Duration
}

// SetupWithManager registers the reconciler.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}
	if r.Connector == nil {
		r.Connector = &connection.ProviderConfigConnector{}
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1alpha1.FirewallFilterMenu{}, providerIndex,
		func(object client.Object) []string {
			menu := object.(*v1alpha1.FirewallFilterMenu)
			if menu.Spec.ProviderConfigRef.Name == "" {
				return nil
			}
			return []string{menu.Spec.ProviderConfigRef.Name}
		}); err != nil {
		return fmt.Errorf("index ProviderConfig references: %w", err)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.FirewallFilterMenu{}).
		Complete(r)
}

// Reconcile implements the menu lifecycle, including explicit delete versus
// orphan behavior.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	menu := &v1alpha1.FirewallFilterMenu{}
	if err := r.Get(ctx, request.NamespacedName, menu); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !menu.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, menu)
	}
	if !controllerutil.ContainsFinalizer(menu, finalizer) {
		before := menu.DeepCopy()
		controllerutil.AddFinalizer(menu, finalizer)
		if err := r.Patch(ctx, menu, client.MergeFrom(before)); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}
	owner, err := r.ownerFor(ctx, menu.Spec.ProviderConfigRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	if owner != "" && owner != menu.Name {
		message := fmt.Sprintf("FirewallFilterMenu %q already owns this ProviderConfig menu", owner)
		if err := r.setStatus(ctx, menu, rest.Plan{}, metav1.ConditionFalse, "OwnershipConflict", message); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.interval()}, nil
	}
	if err := r.trackProviderConfigUsage(ctx, menu); err != nil {
		statusErr := r.setStatus(ctx, menu, rest.Plan{}, metav1.ConditionFalse, "UsageTrackingError", err.Error())
		return ctrl.Result{}, errors.Join(err, statusErr)
	}

	remote, err := r.connect(ctx, menu.Spec.ProviderConfigRef.Name)
	if err != nil {
		statusErr := r.setStatus(ctx, menu, rest.Plan{}, metav1.ConditionFalse, "ConnectionError", err.Error())
		return ctrl.Result{}, errors.Join(err, statusErr)
	}

	desired := make([]rest.Record, len(menu.Spec.Rows))
	for i := range menu.Spec.Rows {
		desired[i] = rest.Record(menu.Spec.Rows[i].Fields())
	}
	spec := rest.MenuSpec{
		Path:     menuPath,
		Ordered:  true,
		Unlisted: restUnlisted(menu.Spec.Unlisted),
	}

	plan, applyErr := remote.Apply(ctx, spec, desired)
	if applyErr != nil {
		statusErr := r.setStatus(ctx, menu, plan, metav1.ConditionFalse, "ApplyError", applyErr.Error())
		return ctrl.Result{}, errors.Join(applyErr, statusErr)
	}
	message := fmt.Sprintf("applied %d operation(s)", len(plan.Steps))
	if plan.Empty() {
		message = "menu is current"
	}
	if err := r.setStatus(ctx, menu, plan, metav1.ConditionTrue, "Available", message); err != nil {
		return ctrl.Result{}, err
	}

	ctrl.LoggerFrom(ctx).Info("reconciled RouterOS firewall filter menu",
		"providerConfig", menu.Spec.ProviderConfigRef.Name,
		"rows", len(desired),
		"operations", plan.Counts())
	return ctrl.Result{RequeueAfter: r.interval()}, nil
}

func restUnlisted(policy v1alpha1.UnlistedPolicy) rest.Unlisted {
	switch policy {
	case v1alpha1.UnlistedTolerate:
		return rest.UnlistedTolerate
	case v1alpha1.UnlistedPrune:
		return rest.UnlistedPrune
	default:
		return ""
	}
}

func (r *Reconciler) reconcileDelete(ctx context.Context, menu *v1alpha1.FirewallFilterMenu) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(menu, finalizer) {
		return ctrl.Result{}, nil
	}
	owner, err := r.ownerFor(ctx, menu.Spec.ProviderConfigRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	// A waiting object must never mutate the active owner's menu merely because
	// it is being deleted with a destructive policy.
	if owner != "" && owner != menu.Name {
		return r.removeFinalizer(ctx, menu)
	}
	policy := menu.Spec.DeletionPolicy
	if policy == "" {
		policy = v1alpha1.DeletionOrphan
	}
	if policy == v1alpha1.DeletionDelete {
		if menu.Spec.Unlisted != v1alpha1.UnlistedPrune {
			return ctrl.Result{}, errors.New("refusing deletionPolicy Delete without unlisted Prune")
		}
		remote, err := r.connect(ctx, menu.Spec.ProviderConfigRef.Name)
		if err != nil {
			return ctrl.Result{}, err
		}
		if _, err := remote.Apply(ctx, deleteAllSpec, nil); err != nil {
			return ctrl.Result{}, fmt.Errorf("delete RouterOS firewall filter rows: %w", err)
		}
	}

	return r.removeFinalizer(ctx, menu)
}

func (r *Reconciler) removeFinalizer(ctx context.Context, menu *v1alpha1.FirewallFilterMenu) (ctrl.Result, error) {
	before := menu.DeepCopy()
	controllerutil.RemoveFinalizer(menu, finalizer)
	if err := r.Patch(ctx, menu, client.MergeFrom(before)); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) connect(ctx context.Context, name string) (connection.Menu, error) {
	if r.Connector == nil {
		r.Connector = &connection.ProviderConfigConnector{}
	}
	return r.Connector.Connect(ctx, r.Client, name)
}

func (r *Reconciler) ownerFor(ctx context.Context, providerConfig string) (string, error) {
	menus := &v1alpha1.FirewallFilterMenuList{}
	if err := r.List(ctx, menus, client.MatchingFields{providerIndex: providerConfig}); err != nil {
		return "", fmt.Errorf("list FirewallFilterMenus for ProviderConfig %q: %w", providerConfig, err)
	}
	var owner *v1alpha1.FirewallFilterMenu
	for i := range menus.Items {
		candidate := &menus.Items[i]
		// A deleting owner keeps ownership until its finalizer is removed. This
		// prevents its successor from reconciling while delete/orphan semantics
		// are still being completed.
		if !candidate.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(candidate, finalizer) {
			continue
		}
		if owner == nil || createdBefore(candidate, owner) {
			owner = candidate
		}
	}
	if owner == nil {
		return "", nil
	}
	return owner.Name, nil
}

func createdBefore(left, right *v1alpha1.FirewallFilterMenu) bool {
	if left.CreationTimestamp.Equal(&right.CreationTimestamp) {
		return left.Name < right.Name
	}
	return left.CreationTimestamp.Before(&right.CreationTimestamp)
}

func (r *Reconciler) trackProviderConfigUsage(ctx context.Context, menu *v1alpha1.FirewallFilterMenu) error {
	if menu.UID == "" {
		return errors.New("FirewallFilterMenu has no UID for ProviderConfig usage tracking")
	}
	key := client.ObjectKey{Name: string(menu.UID)}
	usage := &clusterv1beta1.ProviderConfigUsage{}
	err := r.Get(ctx, key, usage)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("get ProviderConfigUsage %q: %w", key.Name, err)
	}

	want := &clusterv1beta1.ProviderConfigUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name: key.Name,
			Labels: map[string]string{
				xpv2.LabelKeyProviderName: menu.Spec.ProviderConfigRef.Name,
			},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(menu,
				v1alpha1.SchemeGroupVersion.WithKind("FirewallFilterMenu"))},
		},
		ProviderConfigUsage: xpv2.ProviderConfigUsage{
			ProviderConfigReference: xpv2.Reference{Name: menu.Spec.ProviderConfigRef.Name},
			ResourceReference: xpv2.TypedReference{
				APIVersion: v1alpha1.SchemeGroupVersion.String(),
				Kind:       "FirewallFilterMenu",
				Name:       menu.Name,
				UID:        menu.UID,
			},
		},
	}
	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, want); err != nil {
			return fmt.Errorf("create ProviderConfigUsage %q: %w", key.Name, err)
		}
		return nil
	}

	before := usage.DeepCopy()
	usage.Labels = want.Labels
	usage.OwnerReferences = want.OwnerReferences
	usage.ProviderConfigUsage = want.ProviderConfigUsage
	if statusesEqualProviderUsage(before, usage) {
		return nil
	}
	if err := r.Patch(ctx, usage, client.MergeFrom(before)); err != nil {
		return fmt.Errorf("update ProviderConfigUsage %q: %w", key.Name, err)
	}
	return nil
}

func statusesEqualProviderUsage(left, right *clusterv1beta1.ProviderConfigUsage) bool {
	return left.ProviderConfigReference.Name == right.ProviderConfigReference.Name &&
		left.ResourceReference == right.ResourceReference &&
		apiequality.Semantic.DeepEqual(left.Labels, right.Labels) &&
		apiequality.Semantic.DeepEqual(left.OwnerReferences, right.OwnerReferences)
}

func (r *Reconciler) setStatus(ctx context.Context, menu *v1alpha1.FirewallFilterMenu, plan rest.Plan, state metav1.ConditionStatus, reason, message string) error {
	before := menu.DeepCopy()
	menu.Status.ObservedGeneration = menu.Generation
	menu.Status.Rows = make([]v1alpha1.FirewallFilterRowStatus, len(menu.Spec.Rows))
	for i := range menu.Spec.Rows {
		menu.Status.Rows[i] = v1alpha1.FirewallFilterRowStatus{Index: int32(i), ID: plan.Matched[i]}
	}
	apimeta.SetStatusCondition(&menu.Status.Conditions, metav1.Condition{
		Type:               conditionReady,
		Status:             state,
		ObservedGeneration: menu.Generation,
		Reason:             reason,
		Message:            message,
	})
	if statusesEqual(before.Status, menu.Status) {
		return nil
	}
	if err := r.Status().Patch(ctx, menu, client.MergeFrom(before)); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func statusesEqual(a, b v1alpha1.FirewallFilterMenuStatus) bool {
	if a.ObservedGeneration != b.ObservedGeneration || len(a.Rows) != len(b.Rows) || len(a.Conditions) != len(b.Conditions) {
		return false
	}
	for i := range a.Rows {
		if a.Rows[i] != b.Rows[i] {
			return false
		}
	}
	for i := range a.Conditions {
		left, right := a.Conditions[i], b.Conditions[i]
		left.LastTransitionTime = metav1.Time{}
		right.LastTransitionTime = metav1.Time{}
		if left != right {
			return false
		}
	}
	return true
}

func (r *Reconciler) interval() time.Duration {
	if r.RequeueAfter > 0 {
		return r.RequeueAfter
	}
	return defaultInterval
}
