package firewallfilter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/sindrip/provider-routeros/rest"

	providerv1alpha1 "github.com/sindrip/provider-routeros/native/api/provider/v1alpha1"
	v1alpha1 "github.com/sindrip/provider-routeros/native/api/v1alpha1"
	"github.com/sindrip/provider-routeros/native/internal/connection"
)

const (
	menuPath        = "/ip/firewall/filter"
	finalizer       = "firewallfiltermenus.ip.routeros.m.sindrip.io/finalizer"
	defaultInterval = time.Minute
	conditionReady  = "Ready"
	providerIndex   = "spec.providerConfigRef.name"
	secretIndex     = "spec.secretRefs"
	// PruneApprovalAnnotation approves exactly the pending plan token reported
	// in status.pendingPlan.approvalToken.
	PruneApprovalAnnotation = "firewallfiltermenus.ip.routeros.m.sindrip.io/approve-prune"
)

var errApprovalStale = errors.New("approved plan changed before apply")

var dynamicIgnore = []rest.Record{
	{"dynamic": "true"},
	{"dynamic": "yes"},
}

var volatileApprovalFields = map[string]bool{
	"bytes":   true,
	"dynamic": true,
	"invalid": true,
	"packets": true,
}

const maxDeletePreviews = 20

var deleteAllStaticSpec = rest.MenuSpec{
	Path:     menuPath,
	Ordered:  true,
	Unlisted: rest.UnlistedPrune,
	Ignore:   dynamicIgnore,
}

// Connector resolves a ProviderConfig into a menu client.
type Connector interface {
	Connect(context.Context, client.Reader, string, string) (connection.Connection, error)
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
			return []string{providerKey(menu.Namespace, menu.Spec.ProviderConfigRef.Name)}
		}); err != nil {
		return fmt.Errorf("index ProviderConfig references: %w", err)
	}
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &providerv1alpha1.ProviderConfig{}, secretIndex,
		providerSecretReferenceKeys); err != nil {
		return fmt.Errorf("index ProviderConfig Secret references: %w", err)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.FirewallFilterMenu{}).
		Watches(&providerv1alpha1.ProviderConfig{}, handler.EnqueueRequestsFromMapFunc(r.requestsForProviderConfig)).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.requestsForSecret)).
		Complete(r)
}

func providerSecretReferenceKeys(object client.Object) []string {
	config := object.(*providerv1alpha1.ProviderConfig)
	names := []string{config.Spec.Credentials.SecretRef.Name}
	if config.Spec.TLS != nil && config.Spec.TLS.CASecretRef != nil {
		names = append(names, config.Spec.TLS.CASecretRef.Name)
	}
	keys := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		keys = append(keys, providerKey(config.Namespace, name))
	}
	return keys
}

func (r *Reconciler) requestsForProviderConfig(ctx context.Context, object client.Object) []reconcile.Request {
	menus := &v1alpha1.FirewallFilterMenuList{}
	key := providerKey(object.GetNamespace(), object.GetName())
	if err := r.List(ctx, menus, client.InNamespace(object.GetNamespace()), client.MatchingFields{providerIndex: key}); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "list firewall menus for ProviderConfig event", "providerConfig", key)
		return nil
	}
	requests := make([]reconcile.Request, len(menus.Items))
	for i := range menus.Items {
		requests[i] = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&menus.Items[i])}
	}
	return requests
}

func (r *Reconciler) requestsForSecret(ctx context.Context, object client.Object) []reconcile.Request {
	configs := &providerv1alpha1.ProviderConfigList{}
	key := providerKey(object.GetNamespace(), object.GetName())
	if err := r.List(ctx, configs, client.InNamespace(object.GetNamespace()), client.MatchingFields{secretIndex: key}); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "list ProviderConfigs for Secret event", "secret", key)
		return nil
	}
	requests := make([]reconcile.Request, 0)
	for i := range configs.Items {
		requests = append(requests, r.requestsForProviderConfig(ctx, &configs.Items[i])...)
	}
	return requests
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
	if err := r.trackProviderConfigUsage(ctx, menu); err != nil {
		statusErr := r.setStatus(ctx, menu, rest.Plan{}, metav1.ConditionFalse, "UsageTrackingError", err.Error(), nil)
		return ctrl.Result{}, errors.Join(err, statusErr)
	}

	connected, err := r.connect(ctx, menu.Namespace, menu.Spec.ProviderConfigRef.Name)
	if err != nil {
		statusErr := r.setStatus(ctx, menu, rest.Plan{}, metav1.ConditionFalse, "ConnectionError", err.Error(), nil)
		return ctrl.Result{}, errors.Join(err, statusErr)
	}
	owner, err := r.ownerFor(ctx, menu, connected.TargetFingerprint)
	if err != nil {
		return ctrl.Result{}, err
	}
	current := client.ObjectKeyFromObject(menu)
	if owner != (client.ObjectKey{}) && owner != current {
		message := fmt.Sprintf("FirewallFilterMenu %q already owns this router menu", owner.String())
		if err := r.setStatus(ctx, menu, rest.Plan{}, metav1.ConditionFalse, "OwnershipConflict", message, nil); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.interval()}, nil
	}

	desired := make([]rest.Record, len(menu.Spec.Rows))
	for i := range menu.Spec.Rows {
		desired[i] = rest.Record(menu.Spec.Rows[i].Fields())
	}
	spec := rest.MenuSpec{
		Path:     menuPath,
		Ordered:  true,
		Unlisted: restUnlisted(menu.Spec.Unlisted),
		Ignore:   dynamicIgnore,
	}
	if spec.Unlisted == rest.UnlistedPrune && (!menu.Status.Adopted || menu.Status.AdoptedConnection != connected.Fingerprint) {
		return r.reconcileAdoption(ctx, menu, connected, spec, desired)
	}

	plan, applyErr := connected.Menu.Apply(ctx, spec, desired)
	if applyErr != nil {
		statusErr := r.setStatus(ctx, menu, plan, metav1.ConditionFalse, "ApplyError", applyErr.Error(), nil)
		return ctrl.Result{}, errors.Join(applyErr, statusErr)
	}
	message := fmt.Sprintf("applied %d operation(s)", len(plan.Steps))
	if plan.Empty() {
		message = "menu is current"
	}
	if err := r.setStatus(ctx, menu, plan, metav1.ConditionTrue, "Available", message, func(status *v1alpha1.FirewallFilterMenuStatus) {
		status.PendingPlan = nil
		if status.AdoptedConnection != connected.Fingerprint {
			status.Adopted = false
		}
	}); err != nil {
		return ctrl.Result{}, err
	}

	ctrl.LoggerFrom(ctx).Info("reconciled RouterOS firewall filter menu",
		"providerConfig", providerKey(menu.Namespace, menu.Spec.ProviderConfigRef.Name),
		"rows", len(desired),
		"operations", plan.Counts())
	return ctrl.Result{RequeueAfter: r.interval()}, nil
}

func (r *Reconciler) reconcileAdoption(ctx context.Context, menu *v1alpha1.FirewallFilterMenu, connected connection.Connection, spec rest.MenuSpec, desired []rest.Record) (ctrl.Result, error) {
	preview, err := connected.Menu.Plan(ctx, spec, desired)
	if err != nil {
		statusErr := r.setStatus(ctx, menu, rest.Plan{}, metav1.ConditionFalse, "PlanError", err.Error(), nil)
		return ctrl.Result{}, errors.Join(err, statusErr)
	}
	pending := pendingPlan(connected.Fingerprint, preview)
	if pending.Deletes > 0 && menu.Annotations[PruneApprovalAnnotation] != pending.ApprovalToken {
		message := fmt.Sprintf("first prune would delete %d static row(s); set annotation %s=%s to approve this exact plan",
			pending.Deletes, PruneApprovalAnnotation, pending.ApprovalToken)
		if err := r.setStatus(ctx, menu, preview, metav1.ConditionFalse, "AdoptionPending", message,
			func(status *v1alpha1.FirewallFilterMenuStatus) {
				status.Adopted = false
				status.PendingPlan = pending
			}); err != nil {
			return ctrl.Result{}, err
		}
		ctrl.LoggerFrom(ctx).Info("waiting for first-prune approval",
			"providerConfig", providerKey(menu.Namespace, menu.Spec.ProviderConfigRef.Name),
			"deletes", pending.Deletes,
			"approvalToken", pending.ApprovalToken)
		return ctrl.Result{RequeueAfter: r.interval()}, nil
	}

	applied, err := connected.Menu.ApplyChecked(ctx, spec, desired, func(fresh rest.Plan) error {
		if planToken(connected.Fingerprint, fresh) != pending.ApprovalToken {
			return errApprovalStale
		}
		return nil
	})
	if errors.Is(err, errApprovalStale) {
		fresh := pendingPlan(connected.Fingerprint, applied)
		message := "the router changed after preview; review and approve the new pending plan"
		result := ctrl.Result{RequeueAfter: r.interval()}
		if fresh.Deletes == 0 {
			message = "the router changed after preview; retrying the new non-destructive plan"
			result = ctrl.Result{Requeue: true}
		}
		if statusErr := r.setStatus(ctx, menu, applied, metav1.ConditionFalse, "AdoptionPending", message,
			func(status *v1alpha1.FirewallFilterMenuStatus) {
				status.Adopted = false
				status.PendingPlan = fresh
			}); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return result, nil
	}
	if err != nil {
		statusErr := r.setStatus(ctx, menu, applied, metav1.ConditionFalse, "ApplyError", err.Error(), nil)
		return ctrl.Result{}, errors.Join(err, statusErr)
	}

	message := fmt.Sprintf("adopted menu and applied %d operation(s)", len(applied.Steps))
	if applied.Empty() {
		message = "adopted menu; no changes were required"
	}
	if err := r.setStatus(ctx, menu, applied, metav1.ConditionTrue, "Available", message,
		func(status *v1alpha1.FirewallFilterMenuStatus) {
			status.Adopted = true
			status.AdoptedConnection = connected.Fingerprint
			status.PendingPlan = nil
		}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: r.interval()}, nil
}

func pendingPlan(connectionFingerprint string, plan rest.Plan) *v1alpha1.FirewallFilterPlanStatus {
	counts := plan.Counts()
	pending := &v1alpha1.FirewallFilterPlanStatus{
		ApprovalToken: planToken(connectionFingerprint, plan),
		Creates:       int32(counts[rest.OpCreate]),
		Updates:       int32(counts[rest.OpUpdate]),
		Deletes:       int32(counts[rest.OpDelete]),
		Moves:         int32(counts[rest.OpMove]),
	}
	for _, step := range plan.Steps {
		if step.Op != rest.OpDelete || len(pending.DeleteRows) >= maxDeletePreviews {
			continue
		}
		pending.DeleteRows = append(pending.DeleteRows, v1alpha1.FirewallFilterDeletePreview{
			ID:      step.ID,
			Chain:   step.Row["chain"],
			Action:  step.Row["action"],
			Comment: step.Row["comment"],
		})
	}
	pending.DeleteRowsTruncated = int32(len(pending.DeleteRows)) < pending.Deletes
	return pending
}

func planToken(connectionFingerprint string, plan rest.Plan) string {
	h := sha256.New()
	writePart := func(value string) {
		_, _ = fmt.Fprintf(h, "%d:%s", len(value), value)
	}
	writePart(connectionFingerprint)
	writePart(fmt.Sprintf("%d", len(plan.Steps)))
	for _, step := range plan.Steps {
		writePart(string(step.Op))
		writePart(step.ID)
		writePart(step.Before)
		writePart(fmt.Sprintf("%d", len(step.Order)))
		for _, id := range step.Order {
			writePart(id)
		}
		keys := slices.Sorted(maps.Keys(step.Row))
		keys = slices.DeleteFunc(keys, func(key string) bool {
			return key == rest.IDField || volatileApprovalFields[key]
		})
		writePart(fmt.Sprintf("%d", len(keys)))
		for _, key := range keys {
			writePart(key)
			writePart(step.Row[key])
		}
	}
	return hex.EncodeToString(h.Sum(nil))
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
	policy := menu.Spec.DeletionPolicy
	if policy == "" {
		policy = v1alpha1.DeletionOrphan
	}
	if policy == v1alpha1.DeletionOrphan {
		return r.removeFinalizer(ctx, menu)
	}
	if menu.Spec.Unlisted != v1alpha1.UnlistedPrune {
		return ctrl.Result{}, errors.New("refusing deletionPolicy Delete without unlisted Prune")
	}
	connected, err := r.connect(ctx, menu.Namespace, menu.Spec.ProviderConfigRef.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	owner, err := r.ownerFor(ctx, menu, connected.TargetFingerprint)
	if err != nil {
		return ctrl.Result{}, err
	}
	// A waiting object must never mutate the active owner's menu merely because
	// it is being deleted with a destructive policy.
	if owner != (client.ObjectKey{}) && owner != client.ObjectKeyFromObject(menu) {
		return r.removeFinalizer(ctx, menu)
	}
	if _, err := connected.Menu.Apply(ctx, deleteAllStaticSpec, nil); err != nil {
		return ctrl.Result{}, fmt.Errorf("delete static RouterOS firewall filter rows: %w", err)
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

func (r *Reconciler) connect(ctx context.Context, namespace, name string) (connection.Connection, error) {
	if r.Connector == nil {
		r.Connector = &connection.ProviderConfigConnector{}
	}
	return r.Connector.Connect(ctx, r.Client, namespace, name)
}

func (r *Reconciler) ownerFor(ctx context.Context, current *v1alpha1.FirewallFilterMenu, target string) (client.ObjectKey, error) {
	menus := &v1alpha1.FirewallFilterMenuList{}
	if err := r.List(ctx, menus); err != nil {
		return client.ObjectKey{}, fmt.Errorf("list FirewallFilterMenus for router ownership: %w", err)
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
		candidateTarget := target
		if client.ObjectKeyFromObject(candidate) != client.ObjectKeyFromObject(current) {
			connected, err := r.connect(ctx, candidate.Namespace, candidate.Spec.ProviderConfigRef.Name)
			if err != nil {
				ctrl.LoggerFrom(ctx).Error(err, "skip unresolved firewall menu during router ownership election",
					"firewallFilterMenu", client.ObjectKeyFromObject(candidate).String())
				continue
			}
			candidateTarget = connected.TargetFingerprint
		}
		if candidateTarget != target {
			continue
		}
		if owner == nil || createdBefore(candidate, owner) {
			owner = candidate
		}
	}
	if owner == nil {
		return client.ObjectKey{}, nil
	}
	return client.ObjectKeyFromObject(owner), nil
}

func providerKey(namespace, name string) string {
	return namespace + "/" + name
}

func createdBefore(left, right *v1alpha1.FirewallFilterMenu) bool {
	if left.CreationTimestamp.Equal(&right.CreationTimestamp) {
		return client.ObjectKeyFromObject(left).String() < client.ObjectKeyFromObject(right).String()
	}
	return left.CreationTimestamp.Before(&right.CreationTimestamp)
}

func (r *Reconciler) trackProviderConfigUsage(ctx context.Context, menu *v1alpha1.FirewallFilterMenu) error {
	if menu.UID == "" {
		return errors.New("FirewallFilterMenu has no UID for ProviderConfig usage tracking")
	}
	key := client.ObjectKey{Namespace: menu.Namespace, Name: string(menu.UID)}
	usage := &providerv1alpha1.ProviderConfigUsage{}
	err := r.Get(ctx, key, usage)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("get ProviderConfigUsage %q: %w", key.Name, err)
	}

	want := &providerv1alpha1.ProviderConfigUsage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
			Labels: map[string]string{
				xpv2.LabelKeyProviderName: menu.Spec.ProviderConfigRef.Name,
				xpv2.LabelKeyProviderKind: providerv1alpha1.ProviderConfigKind,
			},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(menu,
				v1alpha1.SchemeGroupVersion.WithKind("FirewallFilterMenu"))},
		},
		TypedProviderConfigUsage: xpv2.TypedProviderConfigUsage{
			ProviderConfigReference: xpv2.ProviderConfigReference{
				Kind: providerv1alpha1.ProviderConfigKind,
				Name: menu.Spec.ProviderConfigRef.Name,
			},
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
	usage.TypedProviderConfigUsage = want.TypedProviderConfigUsage
	if statusesEqualProviderUsage(before, usage) {
		return nil
	}
	if err := r.Patch(ctx, usage, client.MergeFrom(before)); err != nil {
		return fmt.Errorf("update ProviderConfigUsage %q: %w", key.Name, err)
	}
	return nil
}

func statusesEqualProviderUsage(left, right *providerv1alpha1.ProviderConfigUsage) bool {
	return left.ProviderConfigReference == right.ProviderConfigReference &&
		left.ResourceReference == right.ResourceReference &&
		apiequality.Semantic.DeepEqual(left.Labels, right.Labels) &&
		apiequality.Semantic.DeepEqual(left.OwnerReferences, right.OwnerReferences)
}

func (r *Reconciler) setStatus(ctx context.Context, menu *v1alpha1.FirewallFilterMenu, plan rest.Plan, state metav1.ConditionStatus, reason, message string, mutate func(*v1alpha1.FirewallFilterMenuStatus)) error {
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
	if mutate != nil {
		mutate(&menu.Status)
	}
	if statusesEqual(before.Status, menu.Status) {
		return nil
	}
	if err := r.Status().Patch(ctx, menu, client.MergeFrom(before)); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func statusesEqual(a, b v1alpha1.FirewallFilterMenuStatus) bool {
	if a.ObservedGeneration != b.ObservedGeneration || a.Adopted != b.Adopted || a.AdoptedConnection != b.AdoptedConnection ||
		!apiequality.Semantic.DeepEqual(a.PendingPlan, b.PendingPlan) || len(a.Rows) != len(b.Rows) || len(a.Conditions) != len(b.Conditions) {
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
