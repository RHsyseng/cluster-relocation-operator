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

	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	"github.com/go-logr/logr"
)

// ClusterRelocationReconciler reconciles a ClusterRelocation object
type ClusterRelocationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

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

	relocation := &rhsysenggithubiov1beta1.ClusterRelocation{}

	err := r.Get(ctx, req.NamespacedName, relocation)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ClusterRelocation resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ClusterRelocation")
		return ctrl.Result{}, err
	}
	defer r.updateStatus(ctx, relocation, logger)

	err = validateCR(relocation)
	if err != nil {
		logger.Error(err, "Could not validate ClusterRelocation")
		return ctrl.Result{}, nil
	} else {
		logger.Info("validation succeeded")
	}

	return ctrl.Result{}, nil
}

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
	err := r.Status().Update(ctx, relocation)
	if err != nil {
		logger.Error(err, "Failed to update Status")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterRelocationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rhsysenggithubiov1beta1.ClusterRelocation{}).
		Complete(r)
}
