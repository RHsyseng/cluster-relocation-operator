package api

import (
	"context"
	"fmt"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	secrets "github.com/RHsyseng/cluster-relocation-operator/internal/secrets"
	"github.com/RHsyseng/cluster-relocation-operator/internal/util"
	"github.com/go-logr/logr"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//+kubebuilder:rbac:groups="",resources=secrets,verbs=create;update;get;list;watch
//+kubebuilder:rbac:groups=config.openshift.io,resources=apiservers,verbs=patch;get;list;watch

func Reconcile(ctx context.Context, c client.Client, scheme *runtime.Scheme, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	var origSecretName string
	var origSecretNamespace string
	if relocation.Spec.APICertRef == nil {
		// If they haven't specified an APICertRef, we generate a self-signed certificate for them
		origSecretName = "generated-api-secret"
		origSecretNamespace = rhsysenggithubiov1beta1.ConfigNamespace
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: origSecretName, Namespace: origSecretNamespace}}

		op, err := controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
			_, ok := secret.Data[corev1.TLSCertKey]
			// Check if the secret already has a key set
			// If there is no key set, generate one
			// This is done so that we don't generate a new certificate each time Reconcile runs
			if !ok {
				logger.Info("generating new TLS cert for API")
				var err error
				secret.Data, err = secrets.GenerateTLSKeyPair(relocation.Spec.Domain, "api")
				if err != nil {
					return err
				}
			} else {
				logger.Info("TLS cert already exists for API")
				commonName, err := secrets.GetCertCommonName(secret.Data[corev1.TLSCertKey])
				if err != nil {
					return err
				}
				if commonName != fmt.Sprintf("api.%s", relocation.Spec.Domain) {
					logger.Info("Domain name has changed, generating new TLS certificate for API")
					var err error
					secret.Data, err = secrets.GenerateTLSKeyPair(relocation.Spec.Domain, "api")
					if err != nil {
						return err
					}
				}
			}
			secret.Type = corev1.SecretTypeTLS
			// Set the controller as the owner so that the secret is deleted along with the CR
			return controllerutil.SetControllerReference(relocation, secret, scheme)
		})
		if err != nil {
			return err
		}
		if op != controllerutil.OperationResultNone {
			logger.Info("Self-signed API TLS cert modified", "OperationResult", op)
		}
	} else {
		if relocation.Spec.APICertRef.Name == "" || relocation.Spec.APICertRef.Namespace == "" {
			return fmt.Errorf("must specify secret name and namespace")
		}
		origSecretName = relocation.Spec.APICertRef.Name
		origSecretNamespace = relocation.Spec.APICertRef.Namespace
		logger.Info("Using user provided API certificate", "namespace", origSecretNamespace, "name", origSecretName)

		// The certificate must be in the openshift-config namespace
		// so if their certificate is in another namespace, we copy it
		if origSecretNamespace != rhsysenggithubiov1beta1.ConfigNamespace {
			secretName := "copied-api-secret"
			// Copy the secret into the openshift-config namespace
			// the original may be owned by another controller (cert-manager for example)
			// we add non-controller ownership to this secret, in order to watch it.
			// our controller should own the destination secret
			copySettings := secrets.SecretCopySettings{
				OwnOriginal:                  true,
				OriginalOwnedByController:    false,
				OwnDestination:               true,
				DestinationOwnedByController: true,
			}
			op, err := secrets.CopySecret(ctx, c, relocation, scheme, origSecretName, origSecretNamespace, secretName, rhsysenggithubiov1beta1.ConfigNamespace, copySettings)
			if err != nil {
				return err
			}
			if op != controllerutil.OperationResultNone {
				logger.Info(fmt.Sprintf("User provided API cert copied to %s", rhsysenggithubiov1beta1.ConfigNamespace), "OperationResult", op)
			}
			origSecretName = secretName
		}
	}

	apiServer := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
	op, err := controllerutil.CreateOrPatch(ctx, c, apiServer, func() error {
		apiServer.Spec.ServingCerts.NamedCertificates = []configv1.APIServerNamedServingCert{
			{
				Names:              []string{fmt.Sprintf("api.%s", relocation.Spec.Domain)},
				ServingCertificate: configv1.SecretNameReference{Name: origSecretName},
			},
		}
		return nil
	})
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		if err := util.WaitForCO(ctx, c, logger, "kube-apiserver", true); err != nil {
			return err
		}
		logger.Info("APIServer modified", "OperationResult", op)
	}
	return nil
}

func Cleanup(ctx context.Context, c client.Client, logger logr.Logger) error {
	// We modified the APIServer resource, but we don't own it
	// Therefore, we need to use a finalizer to put it back the way we found it if the CR is deleted
	apiServer := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
	op, err := controllerutil.CreateOrPatch(ctx, c, apiServer, func() error {
		apiServer.Spec.ServingCerts.NamedCertificates = nil
		return nil
	})
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		// if we let the finalizer finish before the API server has updated, it will delete a MachineConfig and cause a reboot
		// if the node reboots before the API server has updated, it can cause the API server to lock up on the next boot
		if err := util.WaitForCO(ctx, c, logger, "kube-apiserver", true); err != nil {
			return err
		}
		logger.Info("APIServer reverted to original state", "OperationResult", op)
	}
	return nil
}
