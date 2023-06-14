/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	reconcileApi "github.com/RHsyseng/cluster-relocation-operator/internal/api"
	reconcileCatalog "github.com/RHsyseng/cluster-relocation-operator/internal/catalog"
	reconcileDns "github.com/RHsyseng/cluster-relocation-operator/internal/dns"
	reconcileIngress "github.com/RHsyseng/cluster-relocation-operator/internal/ingress"
	reconcileMirror "github.com/RHsyseng/cluster-relocation-operator/internal/mirror"
	reconcilePullSecret "github.com/RHsyseng/cluster-relocation-operator/internal/pullSecret"
	registryCert "github.com/RHsyseng/cluster-relocation-operator/internal/registryCert"
	reconcileSsh "github.com/RHsyseng/cluster-relocation-operator/internal/ssh"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	machineconfigurationv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	operatorhubv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"golang.org/x/mod/semver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// ClusterRelocationReconciler reconciles a ClusterRelocation object
type ClusterRelocationReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Ctrl         controller.Controller
	WatchingIDMS bool
}

const relocationFinalizer = "relocationfinalizer"

//+kubebuilder:rbac:groups=rhsyseng.github.io,resources=clusterrelocations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rhsyseng.github.io,resources=clusterrelocations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rhsyseng.github.io,resources=clusterrelocations/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=secrets,verbs=watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=watch
//+kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get
//+kubebuilder:rbac:groups=config.openshift.io,resources=imagedigestmirrorsets,verbs=watch
//+kubebuilder:rbac:groups=operators.coreos.com,resources=catalogsources,verbs=watch
//+kubebuilder:rbac:groups=machineconfiguration.openshift.io,resources=machineconfigs,verbs=watch
//+kubebuilder:rbac:groups=operator.openshift.io,resources=imagecontentsourcepolicies,verbs=watch

func (r *ClusterRelocationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	relocation := &rhsysenggithubiov1beta1.ClusterRelocation{}

	if err := r.Get(ctx, req.NamespacedName, relocation); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ClusterRelocation resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ClusterRelocation")
		return ctrl.Result{}, err
	}

	// Check if the ClusterRelocation instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isClusterRelocationMarkedToBeDeleted := relocation.GetDeletionTimestamp() != nil
	if isClusterRelocationMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(relocation, relocationFinalizer) {
			// Run finalization logic for relocationFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			requeue, err := r.finalizeRelocation(ctx, logger, relocation)
			if err != nil {
				return ctrl.Result{}, err
			}
			if requeue {
				return ctrl.Result{RequeueAfter: time.Second * 20}, nil
			}

			// Remove relocationFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(relocation, relocationFinalizer)
			if err := r.Update(ctx, relocation); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info("Finalizer removed")
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(relocation, relocationFinalizer) {
		controllerutil.AddFinalizer(relocation, relocationFinalizer)
		if err := r.Update(ctx, relocation); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("Added finalizer to CR")
	}

	defer r.updateStatus(ctx, relocation, logger)

	// This ensures that the user created a valid CR
	// before trying to run through the reconciliation
	if err := validateCR(relocation); err != nil {
		logger.Error(err, "Could not validate ClusterRelocation")
		return ctrl.Result{}, nil
	}
	logger.Info("validation succeeded")

	clusterVersion := &configv1.ClusterVersion{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: "version"}, clusterVersion); err != nil {
		return ctrl.Result{}, err
	}
	clusterVersionString := fmt.Sprintf("v%s", clusterVersion.Status.Desired.Version)

	if !r.WatchingIDMS {
		if semver.Compare(clusterVersionString, "v4.12.999") == 1 {
			// This has to be done dynamically because ImageDigestMirrorSet only exists on OCP 4.13+
			if err := r.Ctrl.Watch(&source.Kind{Type: &configv1.ImageDigestMirrorSet{}}, &handler.EnqueueRequestForOwner{OwnerType: &rhsysenggithubiov1beta1.ClusterRelocation{}, IsController: true}); err != nil {
				return ctrl.Result{}, err
			}
		}
		r.WatchingIDMS = true
	}

	// Applies a new certificate and domain alias to the API server
	if err := reconcileApi.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		apiCondition := metav1.Condition{
			Status:             metav1.ConditionFalse,
			Reason:             rhsysenggithubiov1beta1.ReconciliationFailedReason,
			Message:            err.Error(),
			Type:               rhsysenggithubiov1beta1.ConditionTypeAPI,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, apiCondition)
		return ctrl.Result{}, err
	}
	apiCondition := metav1.Condition{
		Status:             metav1.ConditionTrue,
		Reason:             rhsysenggithubiov1beta1.ReconciliationSucceededReason,
		Type:               rhsysenggithubiov1beta1.ConditionTypeAPI,
		ObservedGeneration: relocation.GetGeneration(),
	}
	apimeta.SetStatusCondition(&relocation.Status.Conditions, apiCondition)

	// Applies a new certificate and domain alias to the Apps ingressesed
	requeue, err := reconcileIngress.Reconcile(ctx, r.Client, r.Scheme, relocation, logger)
	if err != nil {
		ingressCondition := metav1.Condition{
			Status:             metav1.ConditionFalse,
			Reason:             rhsysenggithubiov1beta1.ReconciliationFailedReason,
			Message:            err.Error(),
			Type:               rhsysenggithubiov1beta1.ConditionTypeIngress,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, ingressCondition)
		return ctrl.Result{}, err
	}
	if requeue {
		return ctrl.Result{RequeueAfter: time.Second * 20}, nil
	}

	ingressCondition := metav1.Condition{
		Status:             metav1.ConditionTrue,
		Reason:             rhsysenggithubiov1beta1.ReconciliationSucceededReason,
		Type:               rhsysenggithubiov1beta1.ConditionTypeIngress,
		ObservedGeneration: relocation.GetGeneration(),
	}
	apimeta.SetStatusCondition(&relocation.Status.Conditions, ingressCondition)

	// Apply a new cluster-wide pull secret
	if err := reconcilePullSecret.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		pullSecretCondition := metav1.Condition{
			Status:             metav1.ConditionFalse,
			Reason:             rhsysenggithubiov1beta1.ReconciliationFailedReason,
			Message:            err.Error(),
			Type:               rhsysenggithubiov1beta1.ConditionTypePullSecret,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, pullSecretCondition)
		return ctrl.Result{}, err
	}
	pullSecretCondition := metav1.Condition{
		Status:             metav1.ConditionTrue,
		Reason:             rhsysenggithubiov1beta1.ReconciliationSucceededReason,
		Type:               rhsysenggithubiov1beta1.ConditionTypePullSecret,
		ObservedGeneration: relocation.GetGeneration(),
	}
	apimeta.SetStatusCondition(&relocation.Status.Conditions, pullSecretCondition)

	// Applies a SSH key for the 'core' user
	if err := reconcileSsh.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		sshCondition := metav1.Condition{
			Status:             metav1.ConditionFalse,
			Reason:             rhsysenggithubiov1beta1.ReconciliationFailedReason,
			Message:            err.Error(),
			Type:               rhsysenggithubiov1beta1.ConditionTypeSSH,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, sshCondition)
		return ctrl.Result{}, err
	}
	sshCondition := metav1.Condition{
		Status:             metav1.ConditionTrue,
		Reason:             rhsysenggithubiov1beta1.ReconciliationSucceededReason,
		Type:               rhsysenggithubiov1beta1.ConditionTypeSSH,
		ObservedGeneration: relocation.GetGeneration(),
	}
	apimeta.SetStatusCondition(&relocation.Status.Conditions, sshCondition)

	// Applies a new registry certificate
	if err := registryCert.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		registryCertCondition := metav1.Condition{
			Status:             metav1.ConditionFalse,
			Reason:             rhsysenggithubiov1beta1.ReconciliationFailedReason,
			Message:            err.Error(),
			Type:               rhsysenggithubiov1beta1.ConditionTypeRegistryCert,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, registryCertCondition)
		return ctrl.Result{}, err
	}
	registryCertCondition := metav1.Condition{
		Status:             metav1.ConditionTrue,
		Reason:             rhsysenggithubiov1beta1.ReconciliationSucceededReason,
		Type:               rhsysenggithubiov1beta1.ConditionTypeRegistryCert,
		ObservedGeneration: relocation.GetGeneration(),
	}
	apimeta.SetStatusCondition(&relocation.Status.Conditions, registryCertCondition)

	// Applies new mirror configuration
	if err := reconcileMirror.Reconcile(ctx, r.Client, r.Scheme, relocation, logger, clusterVersionString); err != nil {
		mirrorCondition := metav1.Condition{
			Status:             metav1.ConditionFalse,
			Reason:             rhsysenggithubiov1beta1.ReconciliationFailedReason,
			Message:            err.Error(),
			Type:               rhsysenggithubiov1beta1.ConditionTypeMirror,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, mirrorCondition)
		return ctrl.Result{}, err
	}
	mirrorCondition := metav1.Condition{
		Status:             metav1.ConditionTrue,
		Reason:             rhsysenggithubiov1beta1.ReconciliationSucceededReason,
		Type:               rhsysenggithubiov1beta1.ConditionTypeMirror,
		ObservedGeneration: relocation.GetGeneration(),
	}
	apimeta.SetStatusCondition(&relocation.Status.Conditions, mirrorCondition)

	// Applies new catalog sources
	if err := reconcileCatalog.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		catalogCondition := metav1.Condition{
			Status:             metav1.ConditionFalse,
			Reason:             rhsysenggithubiov1beta1.ReconciliationFailedReason,
			Message:            err.Error(),
			Type:               rhsysenggithubiov1beta1.ConditionTypeCatalog,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, catalogCondition)
		return ctrl.Result{}, err
	}
	catalogCondition := metav1.Condition{
		Status:             metav1.ConditionTrue,
		Reason:             rhsysenggithubiov1beta1.ReconciliationSucceededReason,
		Type:               rhsysenggithubiov1beta1.ConditionTypeCatalog,
		ObservedGeneration: relocation.GetGeneration(),
	}
	apimeta.SetStatusCondition(&relocation.Status.Conditions, catalogCondition)

	// Adds new internal DNS records
	if err := reconcileDns.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		dnsCondition := metav1.Condition{
			Status:             metav1.ConditionFalse,
			Reason:             rhsysenggithubiov1beta1.ReconciliationFailedReason,
			Message:            err.Error(),
			Type:               rhsysenggithubiov1beta1.ConditionTypeDNS,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, dnsCondition)
		return ctrl.Result{}, err
	}
	dnsCondition := metav1.Condition{
		Status:             metav1.ConditionTrue,
		Reason:             rhsysenggithubiov1beta1.ReconciliationSucceededReason,
		Type:               rhsysenggithubiov1beta1.ConditionTypeDNS,
		ObservedGeneration: relocation.GetGeneration(),
	}
	apimeta.SetStatusCondition(&relocation.Status.Conditions, dnsCondition)

	return ctrl.Result{}, nil
}

// We ensure that the CR is named "cluster"
// This makes it so that only 1 CR is reconciled per cluster (since the CR is cluster-scoped)
func validateCR(relocation *rhsysenggithubiov1beta1.ClusterRelocation) error {
	if relocation.Name != "cluster" {
		err := fmt.Errorf("invalid name: %s. CR name must be: cluster", relocation.Name)
		readyCondition := metav1.Condition{
			Status:             metav1.ConditionFalse,
			Reason:             rhsysenggithubiov1beta1.ValidationFailedReason,
			Message:            err.Error(),
			Type:               rhsysenggithubiov1beta1.ConditionTypeReady,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, readyCondition)
		return err
	}

	readyCondition := metav1.Condition{
		Status:             metav1.ConditionTrue,
		Reason:             rhsysenggithubiov1beta1.ValidationSucceededReason,
		Type:               rhsysenggithubiov1beta1.ConditionTypeReady,
		ObservedGeneration: relocation.GetGeneration(),
	}
	apimeta.SetStatusCondition(&relocation.Status.Conditions, readyCondition)
	return nil
}

func (r *ClusterRelocationReconciler) updateStatus(ctx context.Context, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) {
	if err := r.Status().Update(ctx, relocation); err != nil {
		logger.Error(err, "Failed to update Status")
	}
}

func (r *ClusterRelocationReconciler) finalizeRelocation(ctx context.Context, logger logr.Logger, relocation *rhsysenggithubiov1beta1.ClusterRelocation) (bool, error) {
	requeue := false
	if err := reconcileApi.Cleanup(ctx, r.Client, logger); err != nil {
		return requeue, err
	}

	requeue, err := reconcileIngress.Cleanup(ctx, r.Client, logger)
	if err != nil {
		return requeue, err
	}

	if err := reconcilePullSecret.Cleanup(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		return requeue, err
	}

	if err := registryCert.Cleanup(ctx, r.Client, logger); err != nil {
		return requeue, err
	}

	logger.Info("Successfully finalized ClusterRelocation")
	return requeue, nil
}

func (r *ClusterRelocationReconciler) installSchemes() error {
	if err := configv1.Install(r.Scheme); err != nil { // Add config.openshift.io/v1 to the scheme
		return err
	}

	if err := operatorv1.Install(r.Scheme); err != nil { // Add operator.openshift.io/v1 to the scheme
		return err
	}

	if err := machineconfigurationv1.Install(r.Scheme); err != nil { // Add machineconfiguration.openshift.io/v1 to the scheme
		return err
	}

	if err := operatorv1alpha1.Install(r.Scheme); err != nil { // Add operator.openshift.io/v1alpha1 to the scheme
		return err
	}

	if err := operatorhubv1alpha1.AddToScheme(r.Scheme); err != nil { // Add operators.coreos.com/v1alpha1 to the scheme
		return err
	}

	if err := routev1.Install(r.Scheme); err != nil { // Add route.openshift.io/v1 to the scheme
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterRelocationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := r.installSchemes(); err != nil {
		return err
	}

	controller, err := ctrl.NewControllerManagedBy(mgr).
		For(&rhsysenggithubiov1beta1.ClusterRelocation{}).
		// for user provided certificates, we set a non-controller ownership in order to watch for changes
		// Owns() only watches for 'IsController: true' ownership, so we need to watch Secrets this way
		// 'IsController: false' watches for all types of ownership (including controller ownership)
		Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{OwnerType: &rhsysenggithubiov1beta1.ClusterRelocation{}, IsController: false}).
		Owns(&corev1.ConfigMap{}).
		Owns(&operatorhubv1alpha1.CatalogSource{}).
		Owns(&machineconfigurationv1.MachineConfig{}).
		Owns(&operatorv1alpha1.ImageContentSourcePolicy{}).
		Build(r)
	if err != nil {
		return err
	}

	r.Ctrl = controller

	return nil
}
