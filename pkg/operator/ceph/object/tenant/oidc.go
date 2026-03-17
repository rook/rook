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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/ceph/object"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// OIDCConfig holds OIDC configuration for the cluster
type OIDCConfig struct {
	IssuerURL   string
	Thumbprints []string
	ClientIDs   []string // Audience values from JWT tokens
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

	// Get the client IDs (audience values) from the cluster
	clientIDs, err := getServiceAccountAudiences(ctx, k8sClient, issuerURL)
	if err != nil {
		logger.Warningf("failed to get service account audiences, using defaults: %v", err)
		// Fallback to common audience values
		clientIDs = []string{issuerURL, "kubernetes.default.svc"}
	}

	config := &OIDCConfig{
		IssuerURL:   issuerURL,
		Thumbprints: thumbprints,
		ClientIDs:   clientIDs,
	}

	logger.Infof("retrieved OIDC config: issuer=%s, clientIDs=%v, thumbprints=%v", config.IssuerURL, config.ClientIDs, config.Thumbprints)
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

// getServiceAccountAudiences retrieves the audience values that will be in service account JWT tokens
func getServiceAccountAudiences(ctx context.Context, k8sClient client.Client, issuerURL string) ([]string, error) {
	// The audience in service account tokens must match what's configured in the projected token volume
	// In our case, we use the issuer URL as the audience (see test-assume-role-pod.yaml)
	// This must match exactly for AssumeRoleWithWebIdentity to work

	// Only return the issuer URL as the audience
	// This matches the "audience" field in the serviceAccountToken projection
	audiences := []string{issuerURL}

	logger.Infof("using service account audience: %v (must match projected token audience)", audiences)
	return audiences, nil
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

// CreateIAMClient creates an AWS IAM client for RGW
func CreateIAMClient(objContext *object.Context, adminOpsContext *object.AdminOpsContext) (*iam.IAM, error) {
	// The IAM API is accessed through the regular RGW S3 endpoint, not the admin ops endpoint
	// The admin ops endpoint (objContext.Endpoint) is for the admin API
	// For IAM operations, we use the same endpoint but the AWS SDK will route to the IAM service
	// RGW IAM API doesn't use regions, but AWS SDK requires one, so we use a placeholder

	// Use the same endpoint as admin ops - RGW handles routing internally
	// The endpoint is the RGW service endpoint which handles both S3 and IAM APIs
	iamEndpoint := objContext.Endpoint

	// Create HTTP client with TLS certificate if available
	httpClient := &http.Client{}
	if len(adminOpsContext.TlsCert) > 0 {
		// Create a certificate pool with the RGW CA certificate
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(adminOpsContext.TlsCert) {
			logger.Warning("failed to append RGW CA certificate to pool, using insecure TLS")
			httpClient.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			}
		} else {
			httpClient.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: caCertPool,
				},
			}
			logger.Debug("configured IAM client with RGW CA certificate")
		}
	} else if strings.HasPrefix(iamEndpoint, "https://") {
		// HTTPS endpoint but no certificate provided - use insecure
		logger.Warning("HTTPS endpoint but no TLS certificate provided, using insecure TLS")
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	// Create AWS config with explicit region handling for RGW
	// RGW IAM API doesn't use regions, but AWS SDK requires one
	// We use "default" as a placeholder since RGW ignores it
	awsConfig := &aws.Config{
		Credentials: credentials.NewStaticCredentials(
			adminOpsContext.AdminOpsUserAccessKey,
			adminOpsContext.AdminOpsUserSecretKey,
			"",
		),
		Endpoint:         aws.String(iamEndpoint),
		Region:           aws.String("default"), // Placeholder region - RGW ignores this
		S3ForcePathStyle: aws.Bool(true),
		DisableSSL:       aws.Bool(strings.HasPrefix(iamEndpoint, "http://")),
		HTTPClient:       httpClient,
	}

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AWS session")
	}

	logger.Infof("created IAM client with endpoint %q and placeholder region 'default' (RGW ignores region)", iamEndpoint)
	return iam.New(sess), nil
}

// CreateOIDCProviderViaAPI creates an OIDC provider using the AWS IAM API
func CreateOIDCProviderViaAPI(iamClient *iam.IAM, issuerURL string, thumbprints []string, clientIDs []string) (string, error) {
	logger.Infof("creating OIDC provider via IAM API for issuer %q", issuerURL)

	// Prepare thumbprint list
	thumbprintList := make([]*string, len(thumbprints))
	for i, tp := range thumbprints {
		thumbprintList[i] = aws.String(tp)
	}

	// Prepare client ID list
	clientIDList := make([]*string, len(clientIDs))
	for i, cid := range clientIDs {
		clientIDList[i] = aws.String(cid)
	}
	if len(clientIDList) == 0 {
		// Default client ID for STS
		clientIDList = []*string{aws.String("sts.amazonaws.com")}
	}

	// Create OIDC provider
	input := &iam.CreateOpenIDConnectProviderInput{
		Url:            aws.String(issuerURL),
		ThumbprintList: thumbprintList,
		ClientIDList:   clientIDList,
	}

	result, err := iamClient.CreateOpenIDConnectProvider(input)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create OIDC provider for issuer %q", issuerURL)
	}

	providerARN := aws.StringValue(result.OpenIDConnectProviderArn)
	logger.Infof("successfully created OIDC provider: %s", providerARN)
	return providerARN, nil
}

// ListOIDCProvidersViaAPI lists OIDC providers using the AWS IAM API
func ListOIDCProvidersViaAPI(iamClient *iam.IAM) ([]string, error) {
	logger.Debug("listing OIDC providers via IAM API")

	result, err := iamClient.ListOpenIDConnectProviders(&iam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list OIDC providers")
	}

	var arns []string
	for _, provider := range result.OpenIDConnectProviderList {
		arns = append(arns, aws.StringValue(provider.Arn))
	}

	return arns, nil
}

// DeleteOIDCProviderViaAPI deletes an OIDC provider using the AWS IAM API
func DeleteOIDCProviderViaAPI(iamClient *iam.IAM, providerARN string) error {
	logger.Infof("deleting OIDC provider %q via IAM API", providerARN)

	input := &iam.DeleteOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(providerARN),
	}

	_, err := iamClient.DeleteOpenIDConnectProvider(input)
	if err != nil {
		return errors.Wrapf(err, "failed to delete OIDC provider %q", providerARN)
	}

	logger.Infof("successfully deleted OIDC provider %q", providerARN)
	return nil
}

// Made with Bob
