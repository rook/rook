/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package tenant

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// OIDCConfig holds OIDC configuration for the cluster
type OIDCConfig struct {
	IssuerURL   string
	Thumbprints []string
	ClientID    string
}

// GetClusterOIDCConfig retrieves the OIDC configuration from the OpenShift cluster
func GetClusterOIDCConfig(ctx context.Context, k8sClient client.Client) (*OIDCConfig, error) {
	logger.Info("retrieving cluster OIDC configuration")

	// Get the service account issuer from the cluster
	// In OpenShift, this is typically the kubernetes service in the default namespace
	issuerURL, err := getServiceAccountIssuer(ctx, k8sClient)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get service account issuer")
	}

	// Get the OIDC thumbprints
	thumbprints, err := getOIDCThumbprints(issuerURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get OIDC thumbprints")
	}

	config := &OIDCConfig{
		IssuerURL:   issuerURL,
		Thumbprints: thumbprints,
		ClientID:    "sts.amazonaws.com", // Standard client ID for STS
	}

	logger.Infof("retrieved OIDC config: issuer=%s, thumbprints=%v", config.IssuerURL, config.Thumbprints)
	return config, nil
}

// getServiceAccountIssuer retrieves the service account issuer URL from the cluster
func getServiceAccountIssuer(ctx context.Context, k8sClient client.Client) (string, error) {
	// Try to get the issuer from the service account issuer discovery document
	// This is typically available at https://kubernetes.default.svc/.well-known/openid-configuration

	// First, try to get it from a ConfigMap if it exists
	cm := &corev1.ConfigMap{}
	err := k8sClient.Get(ctx, client.ObjectKey{
		Namespace: "kube-public",
		Name:      "cluster-info",
	}, cm)

	if err == nil {
		if issuer, ok := cm.Data["issuer"]; ok {
			return issuer, nil
		}
	}

	// Fallback to the default Kubernetes service
	// In OpenShift, this is typically https://kubernetes.default.svc
	return "https://kubernetes.default.svc", nil
}

// getOIDCThumbprints retrieves the certificate thumbprints for the OIDC issuer
func getOIDCThumbprints(issuerURL string) ([]string, error) {
	logger.Infof("retrieving OIDC thumbprints for issuer %s", issuerURL)

	// Parse the issuer URL to get the host
	host := strings.TrimPrefix(issuerURL, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.Split(host, "/")[0]

	// Connect to the host and get the certificate chain
	conn, err := tls.Dial("tcp", host+":443", &tls.Config{
		InsecureSkipVerify: true, // We only need the cert, not to validate it
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to %s", host)
	}
	defer conn.Close()

	// Get the certificate chain
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, errors.New("no certificates found in chain")
	}

	// Calculate thumbprints for all certificates in the chain
	var thumbprints []string
	for _, cert := range certs {
		thumbprint := calculateThumbprint(cert)
		thumbprints = append(thumbprints, thumbprint)
		logger.Debugf("certificate thumbprint: %s (subject: %s)", thumbprint, cert.Subject)
	}

	return thumbprints, nil
}

// calculateThumbprint calculates the SHA-1 thumbprint of a certificate
func calculateThumbprint(cert *x509.Certificate) string {
	hash := sha1.Sum(cert.Raw)
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

// VerifyOIDCConfiguration verifies that the OIDC configuration is valid
func VerifyOIDCConfiguration(issuerURL string) error {
	logger.Infof("verifying OIDC configuration for issuer %s", issuerURL)

	// Try to fetch the OIDC discovery document
	discoveryURL := fmt.Sprintf("%s/.well-known/openid-configuration", issuerURL)

	resp, err := http.Get(discoveryURL)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch OIDC discovery document from %s", discoveryURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("OIDC discovery document returned status %d", resp.StatusCode)
	}

	logger.Infof("OIDC configuration verified for issuer %s", issuerURL)
	return nil
}

// CreateServiceAccountWithOIDCAnnotations creates a service account with OIDC annotations
func CreateServiceAccountWithOIDCAnnotations(ctx context.Context, k8sClient client.Client, namespace, accountName, roleARN string) error {
	logger.Infof("creating service account with OIDC annotations in namespace %s", namespace)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rgw-identity",
			Namespace: namespace,
			Annotations: map[string]string{
				"object.fusion.io/account-id": accountName,
				"eks.amazonaws.com/role-arn":  roleARN, // Standard annotation for IRSA
			},
		},
	}

	err := k8sClient.Create(ctx, sa)
	if err != nil {
		return errors.Wrapf(err, "failed to create service account in namespace %s", namespace)
	}

	logger.Infof("created service account with OIDC annotations in namespace %s", namespace)
	return nil
}

// GetServiceAccountToken retrieves the JWT token for a service account
func GetServiceAccountToken(ctx context.Context, k8sClient client.Client, namespace, serviceAccountName string) (string, error) {
	logger.Debugf("retrieving token for service account %s in namespace %s", serviceAccountName, namespace)

	// Get the service account
	sa := &corev1.ServiceAccount{}
	err := k8sClient.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      serviceAccountName,
	}, sa)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get service account %s", serviceAccountName)
	}

	// In Kubernetes 1.24+, tokens are not automatically created
	// We would need to create a TokenRequest or use a bound service account token
	// For now, return a placeholder
	logger.Debugf("service account %s found in namespace %s", serviceAccountName, namespace)
	return "", errors.New("token retrieval not yet implemented - use projected volume tokens")
}

// Made with Bob
