package pullSecret

import (
	"context"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	secrets "github.com/RHsyseng/cluster-relocation-operator/internal/secrets"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func Reconcile(client client.Client, scheme *runtime.Scheme, ctx context.Context, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	backupPullSecret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Name: rhsysenggithubiov1beta1.BackupPullSecretName, Namespace: rhsysenggithubiov1beta1.ConfigNamespace}, backupPullSecret); err != nil {
		if errors.IsNotFound(err) {
			// if we haven't yet made a backup of the original pull secret, make one now
			op, err := secrets.CopySecret(ctx, client, relocation, scheme, rhsysenggithubiov1beta1.PullSecretName, rhsysenggithubiov1beta1.ConfigNamespace, rhsysenggithubiov1beta1.BackupPullSecretName, rhsysenggithubiov1beta1.ConfigNamespace)
			if err != nil {
				return err
			}
			if op != controllerutil.OperationResultNone {
				logger.Info("Made a backup of the original pull secret", "OperationResult", op)
			}
		} else {
			return err
		}
	}
	if relocation.Spec.PullSecretRef.Name == "" {
		// if they don't specify a new pull secret, nothing to do
		return nil
	}
	op, err := secrets.CopySecret(ctx, client, relocation, scheme, relocation.Spec.PullSecretRef.Name, relocation.Spec.PullSecretRef.Namespace, rhsysenggithubiov1beta1.PullSecretName, rhsysenggithubiov1beta1.ConfigNamespace)
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		logger.Info("Updated pull secret", "OperationResult", op)
	}
	return nil
}
