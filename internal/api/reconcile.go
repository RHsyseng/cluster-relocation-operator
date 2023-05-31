package api

import (
	"context"
	"fmt"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	certs "github.com/RHsyseng/cluster-relocation-operator/internal/certs"
	"github.com/go-logr/logr"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const secretName = "api-secret"
const secretNamespace = "openshift-config"

func Reconcile(client client.Client, scheme *runtime.Scheme, ctx context.Context, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {
	var origSecretName string
	var origSecretNamespace string
	if relocation.Spec.ApiCertRef.Name == "" {
		// If they haven't specified an ApiCertRef, we generate a self-signed certificate for them
		origSecretName = secretName
		origSecretNamespace = secretNamespace
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: origSecretName, Namespace: origSecretNamespace}}

		op, err := controllerutil.CreateOrUpdate(ctx, client, secret, func() error {
			_, ok := secret.Data[corev1.TLSPrivateKeyKey]
			// Check if the secret already has a key set
			// If there is no key set, generate one
			// This is done so that we don't generate a new certificate each time Reconcile runs
			if !ok {
				logger.Info("generating new TLS cert for API")
				certData, keyData, err := certs.GenerateTLSKeyPair(relocation.Spec.Domain)
				if err != nil {
					return err
				}
				secret.Data = map[string][]byte{
					corev1.TLSCertKey:       certData,
					corev1.TLSPrivateKeyKey: keyData,
				}
			} else {
				logger.Info("TLS cert already exists for API")
			}
			secret.Type = corev1.SecretTypeTLS
			err := controllerutil.SetControllerReference(relocation, secret, scheme)
			return err
		})
		if err != nil {
			return err
		}
		if op != controllerutil.OperationResultNone {
			logger.Info("Self-signed TLS cert modified", "OperationResult", op)
		}
	} else {
		origSecretName = relocation.Spec.ApiCertRef.Name
		origSecretNamespace = relocation.Spec.ApiCertRef.Namespace
		logger.Info("Using user provided API certificate", "namespace", origSecretNamespace, "name", origSecretName)
	}

	if origSecretNamespace != secretNamespace {
		// Copy the secret into openshift-config/api-secret
		op, err := copySecret(ctx, client, relocation, scheme, origSecretName, origSecretNamespace)
		if err != nil {
			return err
		}
		if op != controllerutil.OperationResultNone {
			logger.Info("User provided cert copied to openshift-config", "OperationResult", op)
		}
		origSecretName = secretName
	}

	apiServer := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
	op, err := controllerutil.CreateOrPatch(ctx, client, apiServer, func() error {
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
		logger.Info("APIServer modified", "OperationResult", op)
	}
	return nil
}

func copySecret(ctx context.Context, client client.Client, relocation *rhsysenggithubiov1beta1.ClusterRelocation, scheme *runtime.Scheme, origSecretName string, origSecretNamespace string) (controllerutil.OperationResult, error) {
	origSecret := &corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{Name: origSecretName, Namespace: origSecretNamespace}, origSecret)
	if err != nil {
		return controllerutil.OperationResultNone, err
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: secretNamespace}}

	op, err := controllerutil.CreateOrUpdate(ctx, client, secret, func() error {
		secret.Data = map[string][]byte{
			corev1.TLSCertKey:       origSecret.Data[corev1.TLSCertKey],
			corev1.TLSPrivateKeyKey: origSecret.Data[corev1.TLSPrivateKeyKey],
		}

		secret.Type = corev1.SecretTypeTLS
		err = controllerutil.SetControllerReference(relocation, secret, scheme)
		return err
	})
	return op, err
}
