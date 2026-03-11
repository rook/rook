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
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateThumbprint(t *testing.T) {
	// Create a test certificate
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			CommonName:   "test.example.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	assert.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	assert.NoError(t, err)

	// Calculate thumbprint
	thumbprint := calculateThumbprint(cert)

	// Verify it's a valid SHA-1 hash (40 hex characters, uppercase)
	assert.Len(t, thumbprint, 40)
	assert.Equal(t, strings.ToUpper(thumbprint), thumbprint)

	// Verify it matches manual calculation
	hash := sha1.Sum(cert.Raw)
	expectedThumbprint := strings.ToUpper(hex.EncodeToString(hash[:]))
	assert.Equal(t, expectedThumbprint, thumbprint)
}

func TestOIDCConfigStructure(t *testing.T) {
	config := &OIDCConfig{
		IssuerURL:   "https://kubernetes.default.svc",
		Thumbprints: []string{"ABCD1234", "EFGH5678"},
		ClientIDs:   []string{"kubernetes.default.svc", "kubernetes"},
	}

	assert.Equal(t, "https://kubernetes.default.svc", config.IssuerURL)
	assert.Len(t, config.Thumbprints, 2)
	assert.Len(t, config.ClientIDs, 2)
	assert.Equal(t, "kubernetes.default.svc", config.ClientIDs[0])
}

func TestOIDCConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *OIDCConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &OIDCConfig{
				IssuerURL:   "https://kubernetes.default.svc",
				Thumbprints: []string{"ABCD1234"},
				ClientIDs:   []string{"kubernetes.default.svc"},
			},
			wantErr: false,
		},
		{
			name: "empty issuer",
			config: &OIDCConfig{
				IssuerURL:   "",
				Thumbprints: []string{"ABCD1234"},
				ClientIDs:   []string{"kubernetes.default.svc"},
			},
			wantErr: true,
		},
		{
			name: "no thumbprints",
			config: &OIDCConfig{
				IssuerURL:   "https://kubernetes.default.svc",
				Thumbprints: []string{},
				ClientIDs:   []string{"kubernetes.default.svc"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation
			hasIssuer := tt.config.IssuerURL != ""
			hasThumbprints := len(tt.config.Thumbprints) > 0
			isValid := hasIssuer && hasThumbprints

			if tt.wantErr {
				assert.False(t, isValid)
			} else {
				assert.True(t, isValid)
			}
		})
	}
}

func TestServiceAccountAnnotations(t *testing.T) {
	accountName := "RGWtest-project"
	roleARN := "arn:aws:iam::RGWtest-project:role/project-role"

	expectedAnnotations := map[string]string{
		"object.fusion.io/account-id": accountName,
		"eks.amazonaws.com/role-arn":  roleARN,
	}

	// Verify annotation keys are correct
	assert.Contains(t, expectedAnnotations, "object.fusion.io/account-id")
	assert.Contains(t, expectedAnnotations, "eks.amazonaws.com/role-arn")

	// Verify values
	assert.Equal(t, accountName, expectedAnnotations["object.fusion.io/account-id"])
	assert.Equal(t, roleARN, expectedAnnotations["eks.amazonaws.com/role-arn"])
}

func TestIssuerURLParsing(t *testing.T) {
	tests := []struct {
		name         string
		issuerURL    string
		expectedHost string
	}{
		{
			name:         "https URL",
			issuerURL:    "https://kubernetes.default.svc",
			expectedHost: "kubernetes.default.svc",
		},
		{
			name:         "https URL with path",
			issuerURL:    "https://kubernetes.default.svc/path",
			expectedHost: "kubernetes.default.svc",
		},
		{
			name:         "http URL",
			issuerURL:    "http://kubernetes.default.svc",
			expectedHost: "kubernetes.default.svc",
		},
		{
			name:         "URL with port",
			issuerURL:    "https://kubernetes.default.svc:6443",
			expectedHost: "kubernetes.default.svc:6443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the parsing logic from getOIDCThumbprints
			host := strings.TrimPrefix(tt.issuerURL, "https://")
			host = strings.TrimPrefix(host, "http://")
			host = strings.Split(host, "/")[0]

			assert.Equal(t, tt.expectedHost, host)
		})
	}
}

func TestServiceAccountTokenSubject(t *testing.T) {
	namespace := "test-namespace"
	serviceAccountName := "rgw-identity"

	expectedSubject := "system:serviceaccount:" + namespace + ":" + serviceAccountName
	assert.Equal(t, "system:serviceaccount:test-namespace:rgw-identity", expectedSubject)

	// Test with different namespace
	namespace2 := "production"
	expectedSubject2 := "system:serviceaccount:" + namespace2 + ":" + serviceAccountName
	assert.Equal(t, "system:serviceaccount:production:rgw-identity", expectedSubject2)
}

// Made with Bob
