package ingress

import (
	"context"
	"fmt"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	secrets "github.com/RHsyseng/cluster-relocation-operator/internal/secrets"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func Reconcile(c client.Client, scheme *runtime.Scheme, ctx context.Context, relocation *rhsysenggithubiov1beta1.ClusterRelocation, logger logr.Logger) error {

	// Configure certificates with the new domain name for the ingress
	var origSecretName string
	var origSecretNamespace string
	if relocation.Spec.IngressCertRef.Name == "" {
		// If they haven't specified an IngressCertRef, we generate a self-signed certificate for them
		origSecretName = "generated-ingress-secret"
		origSecretNamespace = rhsysenggithubiov1beta1.ConfigNamespace
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: origSecretName, Namespace: origSecretNamespace}}

		op, err := controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
			_, ok := secret.Data[corev1.TLSCertKey]
			// Check if the secret already has a key set
			// If there is no key set, generate one
			// This is done so that we don't generate a new certificate each time Reconcile runs
			if !ok {
				logger.Info("generating new TLS cert for Ingresses")
				var err error
				secret.Data, err = secrets.GenerateTLSKeyPair(relocation.Spec.Domain, "*.apps")
				if err != nil {
					return err
				}
			} else {
				logger.Info("TLS cert already exists for Ingresses")
				commonName, err := secrets.GetCertCommonName(secret.Data[corev1.TLSCertKey])
				if err != nil {
					return err
				}
				if commonName != fmt.Sprintf("*.apps.%s", relocation.Spec.Domain) {
					logger.Info("Domain name has changed, generating new TLS certificate for Ingresses")
					var err error
					secret.Data, err = secrets.GenerateTLSKeyPair(relocation.Spec.Domain, "*.apps")
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
			logger.Info("Self-signed TLS cert modified", "OperationResult", op)
		}
	} else {
		origSecretName = relocation.Spec.IngressCertRef.Name
		origSecretNamespace = relocation.Spec.IngressCertRef.Namespace
		logger.Info("Using user provided Ingress certificate", "namespace", origSecretNamespace, "name", origSecretName)

		// The certificate must be in the openshift-config namespace
		// so if their certificate is in another namespace, we copy it
		if origSecretNamespace != rhsysenggithubiov1beta1.ConfigNamespace {
			secretName := "copied-ingress-secret"
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
				logger.Info(fmt.Sprintf("User provided cert copied to %s", rhsysenggithubiov1beta1.ConfigNamespace), "OperationResult", op)
			}
			origSecretName = secretName

			// Copy cert secret to openshift-ingress namespace for ingress alias update

			destSecreteNamespace := "openshift-ingress"
			op, err = secrets.CopySecret(ctx, c, relocation, scheme, origSecretName, origSecretNamespace, secretName, destSecreteNamespace, copySettings)
			if err != nil {
				return err
			}
			if op != controllerutil.OperationResultNone {
				logger.Info(fmt.Sprintf("User provided cert copied to %s", rhsysenggithubiov1beta1.ConfigNamespace), "OperationResult", op)
			}
		}
	}
	// Define the IngressController's namespace and name
	namespace := "openshift-ingress-operator"
	name := "default"

	ingressController := &operatorv1.IngressController{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       operatorv1.IngressControllerSpec{},
		Status:     operatorv1.IngressControllerStatus{},
	}
	op, err := controllerutil.CreateOrPatch(ctx, c, ingressController, func() error {
		ingressController.Spec.DefaultCertificate = &corev1.LocalObjectReference{Name: origSecretName}
		return nil
	})
	if err != nil {
		return err
	}

	if op != controllerutil.OperationResultNone {
		logger.Info("IngressController modified", "OperationResult", op)
	}

	ingress := &configv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec:       configv1.IngressSpec{},
		Status:     configv1.IngressStatus{},
	}
	op, err = controllerutil.CreateOrPatch(ctx, c, ingress, func() error {
		ingress.Spec.AppsDomain = relocation.Spec.Domain
		ingress.Spec.ComponentRoutes = []configv1.ComponentRouteSpec{
			{
				Hostname:  configv1.Hostname(fmt.Sprintf("console-openshift-console.apps.%s", relocation.Spec.Domain)),
				Name:      "console",
				Namespace: "openshift-console",
				ServingCertKeyPairSecret: configv1.SecretNameReference{
					Name: origSecretName,
				},
			},
			{
				Hostname:  configv1.Hostname(fmt.Sprintf("downloads-openshift-console.apps.%s", relocation.Spec.Domain)),
				Name:      "downloads",
				Namespace: "openshift-console",
				ServingCertKeyPairSecret: configv1.SecretNameReference{
					Name: origSecretName,
				},
			},
			{
				Hostname:  configv1.Hostname(fmt.Sprintf("oauth-openshift.apps.%s", relocation.Spec.Domain)),
				Name:      "oauth-openshift",
				Namespace: "openshift-authentication",
				ServingCertKeyPairSecret: configv1.SecretNameReference{
					Name: origSecretName,
				},
			},
		}
		return err
	})
	if err != nil {
		return err
	}

	if op != controllerutil.OperationResultNone {
		logger.Info("Ingress domain aliases modified", "OperationResult", op)
	}

	return nil
}
