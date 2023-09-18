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

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;create;update;list;watch

func GetCertCommonName(TLSCertKey []byte) (string, error) {
	block, _ := pem.Decode(TLSCertKey)
	if block == nil {
		return "", fmt.Errorf("failed to decode certificate")
	}

	// Parse the certificate
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", err
	}

	return cert.Subject.CommonName, nil
}

func GenerateTLSKeyPair(ctx context.Context, c client.Client, domain string, prefix string) (map[string][]byte, error) {
	// Sign the certificate using loadbalancer-serving-signer
	lbSigningSecret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: "loadbalancer-serving-signer", Namespace: "openshift-kube-apiserver-operator"}, lbSigningSecret); err != nil {
		return nil, err
	}
	certBlock, _ := pem.Decode(lbSigningSecret.Data[corev1.TLSCertKey])
	if certBlock == nil {
		return nil, fmt.Errorf("could not decode loadbalancer-serving-signer certificate")
	}
	lbSigningCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, err
	}
	keyBlock, _ := pem.Decode(lbSigningSecret.Data[corev1.TLSPrivateKeyKey])
	if keyBlock == nil {
		return nil, fmt.Errorf("could not decode loadbalancer-serving-signer private key")
	}
	lbSigningPrivateKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}

	// Generate a private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	// Create a certificate template
	certificateTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: fmt.Sprintf("%s.%s", prefix, domain)},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(3650 * 24 * time.Hour), // Valid for 10 years
		BasicConstraintsValid: true,
		DNSNames:              []string{fmt.Sprintf("%s.%s", prefix, domain)},
	}

	// Create a certificate using the private key and certificate template, signed by loadbalancer-serving-signer
	derBytes, err := x509.CreateCertificate(rand.Reader, &certificateTemplate, lbSigningCert, &privateKey.PublicKey, lbSigningPrivateKey)
	if err != nil {
		return nil, err
	}

	// Create PEM blocks for the certificate and private key
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	return map[string][]byte{
		corev1.TLSCertKey:       certificatePEM,
		corev1.TLSPrivateKeyKey: privateKeyPEM,
	}, nil
}

// copies a secret from one location to another
func CopySecret(ctx context.Context, c client.Client, relocation *rhsysenggithubiov1beta1.ClusterRelocation, scheme *runtime.Scheme,
	origSecretName string, origSecretNamespace string, destSecretName string, destSecretNamespace string, settings SecretCopySettings,
) (controllerutil.OperationResult, error) {
	origSecret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: origSecretName, Namespace: origSecretNamespace}, origSecret); err != nil {
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
		if err := c.Update(ctx, origSecret); err != nil {
			return controllerutil.OperationResultNone, err
		}
	}

	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: destSecretName, Namespace: destSecretNamespace}}
	op, err := controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
		secret.Data = origSecret.Data
		secret.Type = origSecret.Type
		if settings.OwnDestination {
			if settings.DestinationOwnedByController {
				if err := controllerutil.SetControllerReference(relocation, secret, scheme); err != nil {
					return err
				}
			} else {
				if err := controllerutil.SetOwnerReference(relocation, secret, scheme); err != nil {
					return err
				}
			}
		}
		return nil
	})

	return op, err
}

func ValidateSecretType(ctx context.Context, c client.Client, ref *corev1.SecretReference, desiredSecretType corev1.SecretType) error {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ref.Namespace}, secret); err != nil {
		return err
	}
	if secret.Type != desiredSecretType {
		return fmt.Errorf("secret %s is type %s, should be %s", secret.Name, secret.Type, desiredSecretType)
	}
	return nil
}
