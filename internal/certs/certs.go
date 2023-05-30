package certs

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

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
