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
	reconcileApi "github.com/RHsyseng/cluster-relocation-operator/internal/api"
	reconcilePullSecret "github.com/RHsyseng/cluster-relocation-operator/internal/pullSecret"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// ClusterRelocationReconciler reconciles a ClusterRelocation object
type ClusterRelocationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const relocationFinalizer = "relocationfinalizer"

//+kubebuilder:rbac:groups=rhsyseng.github.io,resources=clusterrelocations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rhsyseng.github.io,resources=clusterrelocations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rhsyseng.github.io,resources=clusterrelocations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ClusterRelocation object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *ClusterRelocationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if err := configv1.Install(r.Scheme); err != nil { // Add config.openshift.io/v1 to the scheme
		logger.Error(err, "Failed to install config.openshift.io/v1")
		return ctrl.Result{}, err
	}

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
	} else {
		logger.Info("validation succeeded")
	}

	// Applies a new certificate and domain alias to the API server
	if err := reconcileApi.Reconcile(r.Client, r.Scheme, ctx, relocation, logger); err != nil {
		apiCondition := metav1.Condition{
			Status:             metav1.ConditionFalse,
			Reason:             rhsysenggithubiov1beta1.ReconciliationFailedReason,
			Message:            err.Error(),
			Type:               rhsysenggithubiov1beta1.ConditionTypeApi,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, apiCondition)
		return ctrl.Result{}, err
	} else {
		apiCondition := metav1.Condition{
			Status:             metav1.ConditionTrue,
			Reason:             rhsysenggithubiov1beta1.ReconciliationSucceededReason,
			Type:               rhsysenggithubiov1beta1.ConditionTypeApi,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, apiCondition)
	}

	// Apply a new cluster-wide pull secret
	if err := reconcilePullSecret.Reconcile(r.Client, r.Scheme, ctx, relocation, logger); err != nil {
		pullSecretCondition := metav1.Condition{
			Status:             metav1.ConditionFalse,
			Reason:             rhsysenggithubiov1beta1.ReconciliationFailedReason,
			Message:            err.Error(),
			Type:               rhsysenggithubiov1beta1.ConditionTypePullSecret,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, pullSecretCondition)
		return ctrl.Result{}, err
	} else {
		pullSecretCondition := metav1.Condition{
			Status:             metav1.ConditionTrue,
			Reason:             rhsysenggithubiov1beta1.ReconciliationSucceededReason,
			Type:               rhsysenggithubiov1beta1.ConditionTypePullSecret,
			ObservedGeneration: relocation.GetGeneration(),
		}
		apimeta.SetStatusCondition(&relocation.Status.Conditions, pullSecretCondition)
	}

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

func (r *ClusterRelocationReconciler) finalizeRelocation(ctx context.Context, logger logr.Logger, relocation *rhsysenggithubiov1beta1.ClusterRelocation) error {
	err := reconcileApi.Cleanup(r.Client, ctx, logger)
	if err != nil {
		return err
	}

	err = reconcilePullSecret.Cleanup(r.Client, r.Scheme, ctx, relocation, logger)
	if err != nil {
		return err
	}

	logger.Info("Successfully finalized ClusterRelocation")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterRelocationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rhsysenggithubiov1beta1.ClusterRelocation{}).
		// for user provided certificates, we set a non-controller ownership in order to watch for changes
		// Owns() only watches for 'IsController: true' ownership, so we need to watch Secrets this way
		// 'IsController: false' watches for all types of ownership (including controller ownership)
		Watches(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{OwnerType: &rhsysenggithubiov1beta1.ClusterRelocation{}, IsController: false}).
		Complete(r)
}
