package pullsecret

import (
	"context"
	"fmt"

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

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;delete;list;watch

func Reconcile(ctx context.Context, c client.Client, scheme *runtime.Scheme, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	if relocation.Spec.PullSecretRef == nil {
		// run Cleanup function in case they are moving from PullSecretRef=<something> to PullSecretRef=<empty>
		return Cleanup(ctx, c, scheme, relocation, logger)
	}

	if relocation.Spec.PullSecretRef.Name == "" || relocation.Spec.PullSecretRef.Namespace == "" {
		return fmt.Errorf("must specify secret name and namespace")
	}

	if err := secrets.ValidateSecretType(ctx, c, relocation.Spec.PullSecretRef, corev1.SecretTypeDockerConfigJson); err != nil {
		return err
	}

	backupPullSecret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: rhsysenggithubiov1beta1.BackupPullSecretName, Namespace: rhsysenggithubiov1beta1.ConfigNamespace}, backupPullSecret); err != nil {
		if errors.IsNotFound(err) {
			// if we haven't yet made a backup of the original pull secret, make one now
			// we should own the backup secret, but not the original pull-secret
			copySettings := secrets.SecretCopySettings{
				OwnOriginal:                  false,
				OriginalOwnedByController:    false,
				OwnDestination:               true,
				DestinationOwnedByController: true,
			}
			op, err := secrets.CopySecret(ctx, c, relocation, scheme, rhsysenggithubiov1beta1.PullSecretName, rhsysenggithubiov1beta1.ConfigNamespace, rhsysenggithubiov1beta1.BackupPullSecretName, rhsysenggithubiov1beta1.ConfigNamespace, copySettings)
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

	// copy their secret to the cluster-wide pull secret
	// we own their secret (non-controller ownership) in order to watch it, but we should not own the cluster-wide pull secret
	copySettings := secrets.SecretCopySettings{
		OwnOriginal:                  true,
		OriginalOwnedByController:    false,
		OwnDestination:               false,
		DestinationOwnedByController: false,
	}
	op, err := secrets.CopySecret(ctx, c, relocation, scheme, relocation.Spec.PullSecretRef.Name, relocation.Spec.PullSecretRef.Namespace, rhsysenggithubiov1beta1.PullSecretName, rhsysenggithubiov1beta1.ConfigNamespace, copySettings)
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		logger.Info("Updated pull secret", "OperationResult", op)
	}
	return nil
}

func Cleanup(ctx context.Context, c client.Client, scheme *runtime.Scheme, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	// If we modified the original pull secret, we need to restore it
	backupPullSecret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: rhsysenggithubiov1beta1.BackupPullSecretName, Namespace: rhsysenggithubiov1beta1.ConfigNamespace}, backupPullSecret); err != nil {
		// if there is no backup, that means we didn't modify the pull-secret. Nothing for us to do
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		// copy the backup pull secret into the cluster-wide pull secret
		// the backup should already be owned by the controller
		// we should not own the cluster-wide pull secret
		copySettings := secrets.SecretCopySettings{}
		op, err := secrets.CopySecret(ctx, c, relocation, scheme, rhsysenggithubiov1beta1.BackupPullSecretName, rhsysenggithubiov1beta1.ConfigNamespace, rhsysenggithubiov1beta1.PullSecretName, rhsysenggithubiov1beta1.ConfigNamespace, copySettings)
		if err != nil {
			return err
		}
		if op != controllerutil.OperationResultNone {
			logger.Info("Restored original pull secret", "OperationResult", op)
		}
		// when run as part of the finalizer, deleting the backup pull-secret isn't required (it will be deleted automatically)
		// however, we also run this if the user moves from PullSecretRef=<something> to PullSecretRef=<empty>
		// we delete the backup so that if they ever move back to PullSecretRef=<something>, a fresh backup is taken
		if err := c.Delete(ctx, backupPullSecret); err != nil {
			return err
		}
		logger.Info("Deleted backup pull secret")
	}
	return nil
}
