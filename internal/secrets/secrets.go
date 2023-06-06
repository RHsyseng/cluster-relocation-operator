package certs

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	rhsysenggithubiov1beta1 "github.com/RHsyseng/cluster-relocation-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type SecretCopySettings struct {
	OwnOriginal                  bool
	OriginalOwnedByController    bool
	OwnDestination               bool
	DestinationOwnedByController bool
}

func GenerateTLSKeyPair(domain string) ([]byte, []byte, error) {
	// Generate a private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// Create a self-signed certificate template
	certificateTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: fmt.Sprintf("api.%s", domain)},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(3650 * 24 * time.Hour), // Valid for 10 years
		BasicConstraintsValid: true,
		DNSNames:              []string{fmt.Sprintf("api.%s", domain)},
	}

	// Create a self-signed certificate using the private key and certificate template
	derBytes, err := x509.CreateCertificate(rand.Reader, &certificateTemplate, &certificateTemplate, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	// Create PEM blocks for the certificate and private key
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	return certificatePEM, privateKeyPEM, nil
}

// copies a secret from one location to another
func CopySecret(ctx context.Context, client client.Client, relocation *rhsysenggithubiov1beta1.ClusterRelocation, scheme *runtime.Scheme,
	origSecretName string, origSecretNamespace string, destSecretName string, destSecretNamespace string, settings SecretCopySettings) (controllerutil.OperationResult, error) {
	origSecret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Name: origSecretName, Namespace: origSecretNamespace}, origSecret); err != nil {
		return controllerutil.OperationResultNone, err
	}

	if settings.OwnOriginal {
		if settings.OriginalOwnedByController {
			if err := controllerutil.SetControllerReference(relocation, origSecret, scheme); err != nil {
				return controllerutil.OperationResultNone, err
			}
		} else {
			if err := controllerutil.SetOwnerReference(relocation, origSecret, scheme); err != nil {
				return controllerutil.OperationResultNone, err
			}
		}
		if err := client.Update(ctx, origSecret); err != nil {
			return controllerutil.OperationResultNone, err
		}
	}

	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: destSecretName, Namespace: destSecretNamespace}}
	op, err := controllerutil.CreateOrUpdate(ctx, client, secret, func() error {
		secret.Data = origSecret.Data
		secret.Type = origSecret.Type
		if settings.OwnDestination {
			var err error
			if settings.DestinationOwnedByController {
				err = controllerutil.SetControllerReference(relocation, secret, scheme)
			} else {
				err = controllerutil.SetOwnerReference(relocation, secret, scheme)
			}
			return err
		}
		return nil
	})

	return op, err
}