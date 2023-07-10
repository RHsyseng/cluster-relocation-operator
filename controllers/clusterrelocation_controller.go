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

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	reconcileACM "github.com/RHsyseng/cluster-relocation-operator/internal/acm"
	reconcileAPI "github.com/RHsyseng/cluster-relocation-operator/internal/api"
	reconcileCatalog "github.com/RHsyseng/cluster-relocation-operator/internal/catalog"
	reconcileDNS "github.com/RHsyseng/cluster-relocation-operator/internal/dns"
	reconcileIngress "github.com/RHsyseng/cluster-relocation-operator/internal/ingress"
	reconcileMirror "github.com/RHsyseng/cluster-relocation-operator/internal/mirror"
	reconcilePullSecret "github.com/RHsyseng/cluster-relocation-operator/internal/pullSecret"
	registryCert "github.com/RHsyseng/cluster-relocation-operator/internal/registryCert"
	reconcileSSH "github.com/RHsyseng/cluster-relocation-operator/internal/ssh"
	agentv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorapiv1 "open-cluster-management.io/api/operator/v1"

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

//+kubebuilder:rbac:groups="",resources=secrets,verbs=watch;list
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=watch;list
//+kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get;watch;list
//+kubebuilder:rbac:groups=config.openshift.io,resources=imagedigestmirrorsets,verbs=watch;list
//+kubebuilder:rbac:groups=operators.coreos.com,resources=catalogsources,verbs=watch;list
//+kubebuilder:rbac:groups=machineconfiguration.openshift.io,resources=machineconfigs,verbs=watch;list
//+kubebuilder:rbac:groups=operator.openshift.io,resources=imagecontentsourcepolicies,verbs=watch;list
//+kubebuilder:rbac:groups=operators.coreos.com,resources=subscriptions,verbs=get;list;watch;delete

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
			if err := r.finalizeRelocation(ctx, logger, relocation); err != nil {
				return ctrl.Result{}, err
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

	reconcileCondition := apimeta.FindStatusCondition(relocation.Status.Conditions, rhsysenggithubiov1beta1.ConditionTypeReconciled)
	if reconcileCondition == nil || reconcileCondition.ObservedGeneration < relocation.GetGeneration() {
		r.setFailedStatus(relocation, rhsysenggithubiov1beta1.InProgressReconciliationFailedReason, "reconcile in progress")
		// requeue so that the status is updated right away
		return ctrl.Result{Requeue: true}, nil
	}

	// Adds new internal DNS records
	if err := reconcileDNS.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		r.setFailedStatus(relocation, rhsysenggithubiov1beta1.DNSReconciliationFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// Applies a new certificate and domain alias to the API server
	if err := reconcileAPI.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		r.setFailedStatus(relocation, rhsysenggithubiov1beta1.APIReconciliationFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// Applies a new certificate and domain alias to the Apps ingressesed
	if err := reconcileIngress.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		r.setFailedStatus(relocation, rhsysenggithubiov1beta1.IngressReconciliationFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// Apply a new cluster-wide pull secret
	if err := reconcilePullSecret.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		r.setFailedStatus(relocation, rhsysenggithubiov1beta1.PullSecretReconciliationFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// Applies a SSH key for the 'core' user
	if err := reconcileSSH.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		r.setFailedStatus(relocation, rhsysenggithubiov1beta1.SSHReconciliationFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// Applies a new registry certificate
	if err := registryCert.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		r.setFailedStatus(relocation, rhsysenggithubiov1beta1.RegistryReconciliationFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// Applies new mirror configuration
	if err := reconcileMirror.Reconcile(ctx, r.Client, r.Scheme, relocation, logger, clusterVersionString); err != nil {
		r.setFailedStatus(relocation, rhsysenggithubiov1beta1.MirrorReconciliationFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// Applies new catalog sources
	if err := reconcileCatalog.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		r.setFailedStatus(relocation, rhsysenggithubiov1beta1.CatalogReconciliationFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// Registers to ACM
	if err := reconcileACM.Reconcile(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
		r.setFailedStatus(relocation, rhsysenggithubiov1beta1.ACMReconciliationFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	successCondition := metav1.Condition{
		Status:             metav1.ConditionTrue,
		Reason:             rhsysenggithubiov1beta1.ReconciliationSucceededReason,
		Message:            "reconcile succeeded",
		Type:               rhsysenggithubiov1beta1.ConditionTypeReconciled,
		ObservedGeneration: relocation.GetGeneration(),
	}
	apimeta.SetStatusCondition(&relocation.Status.Conditions, successCondition)

	logger.Info("Reconcile complete")

	if r.isSelfDestructSet(relocation) {
		logger.Info("self-destruct is set, removing CR")
		if err := r.Client.Delete(ctx, relocation, client.PropagationPolicy(metav1.DeletePropagationOrphan)); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *ClusterRelocationReconciler) setFailedStatus(relocation *rhsysenggithubiov1beta1.ClusterRelocation, reason string, message string) {
	failedCondition := metav1.Condition{
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		Type:               rhsysenggithubiov1beta1.ConditionTypeReconciled,
		ObservedGeneration: relocation.GetGeneration(),
	}
	apimeta.SetStatusCondition(&relocation.Status.Conditions, failedCondition)
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

func (r *ClusterRelocationReconciler) finalizeRelocation(ctx context.Context, logger logr.Logger, relocation *rhsysenggithubiov1beta1.ClusterRelocation) error {
	logger.Info("Starting finalizer")

	if r.isSelfDestructSet(relocation) {
		subscriptions := &operatorhubv1alpha1.SubscriptionList{}
		if err := r.Client.List(ctx, subscriptions, client.InNamespace("openshift-operators")); err != nil {
			return err
		}
		for _, v := range subscriptions.Items {
			if v.Spec.Package == "cluster-relocation-operator" {
				if err := r.Client.Delete(ctx, &v); err != nil {
					return err
				}
				logger.Info("operator subscription deleted")
			}
		}
	} else {
		if err := reconcilePullSecret.Cleanup(ctx, r.Client, r.Scheme, relocation, logger); err != nil {
			return err
		}

		if err := registryCert.Cleanup(ctx, r.Client, logger); err != nil {
			return err
		}

		if err := reconcileIngress.Cleanup(ctx, r.Client, logger); err != nil {
			return err
		}

		if err := reconcileAPI.Cleanup(ctx, r.Client, logger); err != nil {
			return err
		}
	}

	logger.Info("Successfully finalized ClusterRelocation")
	return nil
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

	if err := clusterv1.Install(r.Scheme); err != nil { // Add cluster.open-cluster-management.io/v1 to the scheme
		return err
	}

	if err := agentv1.SchemeBuilder.AddToScheme(r.Scheme); err != nil { // Add agent.open-cluster-management.io/v1 to the scheme
		return err
	}

	if err := operatorapiv1.Install(r.Scheme); err != nil { // Add operator.open-cluster-management.io/v1 to the scheme
		return err
	}
	return nil
}

func (r *ClusterRelocationReconciler) isSelfDestructSet(relocation *rhsysenggithubiov1beta1.ClusterRelocation) bool {
	selfDestruct := false
	val, ok := relocation.Annotations["self-destruct"]
	if ok {
		if val == "true" {
			selfDestruct = true
		}
	}
	return selfDestruct
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
