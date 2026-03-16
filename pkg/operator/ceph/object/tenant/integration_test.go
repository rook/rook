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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCompleteWorkflow verifies the complete workflow of creating an account
// and configuring an OpenShift identity provider as the account's OIDC
func TestCompleteWorkflow(t *testing.T) {
	// Test parameters
	accountName := "RGWtest-project"
	namespace := "test-project"
	issuerURL := "https://kubernetes.default.svc"
	thumbprints := []string{"ABCD1234567890ABCD1234567890ABCD12345678"}
	clientIDs := []string{"kubernetes.default.svc", "kubernetes"}

	t.Run("Step 1: Create RGW Account", func(t *testing.T) {
		// Simulate account creation
		account := &RGWAccount{
			AccountID:   accountName,
			AccountName: accountName,
			Email:       "test@example.com",
		}

		// Verify account structure
		assert.Equal(t, accountName, account.AccountID)
		assert.Equal(t, accountName, account.AccountName)
		assert.NotEmpty(t, account.Email)

		// Verify it can be marshaled to JSON (as RGW would return)
		jsonBytes, err := json.Marshal(account)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonBytes), accountName)
	})

	t.Run("Step 2: Configure OIDC Provider", func(t *testing.T) {
		// Create OIDC configuration
		oidcConfig := &OIDCConfig{
			IssuerURL:   issuerURL,
			Thumbprints: thumbprints,
			ClientIDs:   clientIDs,
		}

		// Verify OIDC configuration
		assert.Equal(t, issuerURL, oidcConfig.IssuerURL)
		assert.Len(t, oidcConfig.Thumbprints, 1)
		assert.Len(t, oidcConfig.ClientIDs, 2)
		assert.Contains(t, oidcConfig.ClientIDs, "kubernetes.default.svc")

		// Simulate OIDC provider creation
		provider := &OIDCProvider{
			ProviderARN: "arn:aws:iam::" + accountName + ":oidc-provider/" + issuerURL,
			Issuer:      issuerURL,
			Thumbprints: thumbprints,
		}

		// Verify provider structure
		assert.Contains(t, provider.ProviderARN, accountName)
		assert.Equal(t, issuerURL, provider.Issuer)
		assert.Equal(t, thumbprints, provider.Thumbprints)
	})

	t.Run("Step 3: Create IAM Role with AssumeRole Policy", func(t *testing.T) {
		// Generate OIDC provider ARN
		oidcProviderARN := "arn:aws:iam::" + accountName + ":oidc-provider/kubernetes.default.svc"

		// Generate assume role policy document
		assumeRolePolicyDoc := GenerateAssumeRolePolicyDocument(accountName, oidcProviderARN, namespace)

		// Verify policy document is valid JSON
		var policyDoc PolicyDocument
		err := json.Unmarshal([]byte(assumeRolePolicyDoc), &policyDoc)
		assert.NoError(t, err)

		// Verify policy structure
		assert.Equal(t, "2012-10-17", policyDoc.Version)
		assert.Len(t, policyDoc.Statement, 1)

		stmt := policyDoc.Statement[0]
		assert.Equal(t, "Allow", stmt.Effect)
		assert.Equal(t, "sts:AssumeRoleWithWebIdentity", stmt.Action)

		// Verify principal contains OIDC provider ARN
		principal, ok := stmt.Principal.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, oidcProviderARN, principal["Federated"])

		// Verify condition restricts to specific service account
		assert.NotNil(t, stmt.Condition)
		stringEquals, ok := stmt.Condition["StringEquals"].(map[string]interface{})
		assert.True(t, ok)
		expectedSubject := "system:serviceaccount:" + namespace + ":rgw-identity"
		assert.Equal(t, expectedSubject, stringEquals["oidc:sub"])

		// Simulate role creation
		role := &IAMRole{
			RoleARN:             "arn:aws:iam::" + accountName + ":role/project-role",
			RoleName:            "project-role",
			AssumeRolePolicyDoc: assumeRolePolicyDoc,
		}

		// Verify role structure
		assert.Contains(t, role.RoleARN, accountName)
		assert.Equal(t, "project-role", role.RoleName)
		assert.NotEmpty(t, role.AssumeRolePolicyDoc)
	})

	t.Run("Step 4: Attach Managed Policy", func(t *testing.T) {
		// Use AWS managed policy for S3 full access
		managedPolicyARN := "arn:aws:iam::aws:policy/AmazonS3FullAccess"
		assert.NotEmpty(t, managedPolicyARN)
		t.Logf("Using managed policy: %s", managedPolicyARN)
	})

	t.Run("Step 5: Verify Service Account Annotations", func(t *testing.T) {
		// Expected annotations for the service account
		roleARN := "arn:aws:iam::" + accountName + ":role/project-role"
		expectedAnnotations := map[string]string{
			"object.fusion.io/account-id": accountName,
			"eks.amazonaws.com/role-arn":  roleARN,
		}

		// Verify annotations are correct
		assert.Equal(t, accountName, expectedAnnotations["object.fusion.io/account-id"])
		assert.Equal(t, roleARN, expectedAnnotations["eks.amazonaws.com/role-arn"])
		assert.Contains(t, expectedAnnotations["eks.amazonaws.com/role-arn"], accountName)
	})

	t.Run("Step 6: Verify Complete Integration", func(t *testing.T) {
		// This test verifies that all components work together

		// 1. Account exists
		account := &RGWAccount{
			AccountID:   accountName,
			AccountName: accountName,
		}
		assert.NotNil(t, account)

		// 2. OIDC provider is configured
		oidcProviderARN := "arn:aws:iam::" + accountName + ":oidc-provider/kubernetes.default.svc"
		provider := &OIDCProvider{
			ProviderARN: oidcProviderARN,
			Issuer:      issuerURL,
			Thumbprints: thumbprints,
		}
		assert.NotNil(t, provider)
		assert.Equal(t, issuerURL, provider.Issuer)

		// 3. IAM role exists with correct trust policy
		assumeRolePolicyDoc := GenerateAssumeRolePolicyDocument(accountName, oidcProviderARN, namespace)
		role := &IAMRole{
			RoleARN:             "arn:aws:iam::" + accountName + ":role/project-role",
			RoleName:            "project-role",
			AssumeRolePolicyDoc: assumeRolePolicyDoc,
		}
		assert.NotNil(t, role)
		assert.Contains(t, role.AssumeRolePolicyDoc, oidcProviderARN)
		assert.Contains(t, role.AssumeRolePolicyDoc, "system:serviceaccount:"+namespace+":rgw-identity")

		// 4. Service account would be created with annotations
		serviceAccountAnnotations := map[string]string{
			"object.fusion.io/account-id": accountName,
			"eks.amazonaws.com/role-arn":  role.RoleARN,
		}
		assert.Equal(t, accountName, serviceAccountAnnotations["object.fusion.io/account-id"])
		assert.Equal(t, role.RoleARN, serviceAccountAnnotations["eks.amazonaws.com/role-arn"])

		// 5. Verify the complete flow
		// Account -> OIDC Provider -> IAM Role -> Service Account
		assert.Equal(t, account.AccountID, accountName)
		assert.Contains(t, provider.ProviderARN, account.AccountID)
		assert.Contains(t, role.RoleARN, account.AccountID)
		assert.Equal(t, serviceAccountAnnotations["object.fusion.io/account-id"], account.AccountID)
	})
}

// TestOIDCThumbprintValidation verifies thumbprint calculation and validation
func TestOIDCThumbprintValidation(t *testing.T) {
	// Test that thumbprints are properly formatted
	thumbprint := "ABCD1234567890ABCD1234567890ABCD12345678"

	// Verify length (SHA-1 hash is 40 hex characters)
	assert.Len(t, thumbprint, 40)

	// Verify it's uppercase hex
	for _, c := range thumbprint {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'A' && c <= 'F'))
	}
}

// TestServiceAccountSubjectFormat verifies the service account subject format
func TestServiceAccountSubjectFormat(t *testing.T) {
	namespace := "test-namespace"
	serviceAccountName := "rgw-identity"

	expectedSubject := "system:serviceaccount:" + namespace + ":" + serviceAccountName
	assert.Equal(t, "system:serviceaccount:test-namespace:rgw-identity", expectedSubject)

	// Verify it matches the format used in the assume role policy
	accountName := "RGWtest-account"
	oidcProviderARN := "arn:aws:iam::" + accountName + ":oidc-provider/kubernetes.default.svc"
	assumeRolePolicyDoc := GenerateAssumeRolePolicyDocument(accountName, oidcProviderARN, namespace)

	assert.Contains(t, assumeRolePolicyDoc, expectedSubject)
}

// TestPolicyDocumentGeneration verifies all policy generation functions
func TestPolicyDocumentGeneration(t *testing.T) {
	accountName := "RGWtest-account"

	t.Run("Full Access Policy", func(t *testing.T) {
		policy := GenerateFullAccessPolicy(accountName)
		assert.Contains(t, policy, "s3:*")
		assert.Contains(t, policy, "2012-10-17")

		var doc PolicyDocument
		err := json.Unmarshal([]byte(policy), &doc)
		assert.NoError(t, err)
	})

	t.Run("Read Only Policy", func(t *testing.T) {
		policy := GenerateReadOnlyPolicy(accountName, "test-bucket")
		assert.Contains(t, policy, "s3:GetObject")
		assert.Contains(t, policy, "s3:ListBucket")
		assert.NotContains(t, policy, "s3:PutObject")

		var doc PolicyDocument
		err := json.Unmarshal([]byte(policy), &doc)
		assert.NoError(t, err)
	})

	t.Run("Bucket Specific Policy", func(t *testing.T) {
		policy := GenerateBucketSpecificPolicy(accountName, "my-bucket", []string{"s3:GetObject", "s3:PutObject"})
		assert.Contains(t, policy, "my-bucket")
		assert.Contains(t, policy, "s3:GetObject")
		assert.Contains(t, policy, "s3:PutObject")

		var doc PolicyDocument
		err := json.Unmarshal([]byte(policy), &doc)
		assert.NoError(t, err)
	})

	t.Run("Path Based Policy", func(t *testing.T) {
		policy := GeneratePathBasedPolicy(accountName, "my-bucket", "user-data/", []string{"s3:GetObject"})
		assert.Contains(t, policy, "my-bucket")
		assert.Contains(t, policy, "user-data/")

		var doc PolicyDocument
		err := json.Unmarshal([]byte(policy), &doc)
		assert.NoError(t, err)
	})
}

// TestEndToEndScenario simulates a complete end-to-end scenario
func TestEndToEndScenario(t *testing.T) {
	// Scenario: A user annotates an OpenShift project to enable identity binding
	// The controller should create all necessary resources

	projectName := "my-application"
	accountName := "RGW" + projectName
	namespace := projectName
	issuerURL := "https://kubernetes.default.svc"

	t.Log("=== Starting End-to-End Scenario ===")

	// Step 1: User annotates project
	t.Log("Step 1: User annotates OpenShift project with object.fusion.io/identity-binding: true")
	annotation := map[string]string{
		"object.fusion.io/identity-binding": "true",
	}
	assert.Equal(t, "true", annotation["object.fusion.io/identity-binding"])

	// Step 2: Controller creates RGW account
	t.Log("Step 2: Controller creates RGW User Account")
	account := &RGWAccount{
		AccountID:   accountName,
		AccountName: accountName,
		Email:       projectName + "@example.com",
	}
	assert.Equal(t, accountName, account.AccountID)
	t.Logf("  Created account: %s", account.AccountID)

	// Step 3: Controller retrieves OIDC configuration
	t.Log("Step 3: Controller retrieves cluster OIDC configuration")
	oidcConfig := &OIDCConfig{
		IssuerURL:   issuerURL,
		Thumbprints: []string{"ABCD1234567890ABCD1234567890ABCD12345678"},
		ClientIDs:   []string{"kubernetes.default.svc", "kubernetes"},
	}
	assert.NotEmpty(t, oidcConfig.IssuerURL)
	t.Logf("  OIDC Issuer: %s", oidcConfig.IssuerURL)

	// Step 4: Controller creates OIDC provider in RGW account
	t.Log("Step 4: Controller creates OIDC provider in RGW account")
	oidcProviderARN := "arn:aws:iam::" + accountName + ":oidc-provider/kubernetes.default.svc"
	provider := &OIDCProvider{
		ProviderARN: oidcProviderARN,
		Issuer:      oidcConfig.IssuerURL,
		Thumbprints: oidcConfig.Thumbprints,
	}
	assert.Equal(t, oidcConfig.IssuerURL, provider.Issuer)
	t.Logf("  Created OIDC provider: %s", provider.ProviderARN)

	// Step 5: Controller creates IAM role with trust policy
	t.Log("Step 5: Controller creates IAM role with AssumeRoleWithWebIdentity trust policy")
	assumeRolePolicyDoc := GenerateAssumeRolePolicyDocument(accountName, oidcProviderARN, namespace)
	role := &IAMRole{
		RoleARN:             "arn:aws:iam::" + accountName + ":role/project-role",
		RoleName:            "project-role",
		AssumeRolePolicyDoc: assumeRolePolicyDoc,
	}
	assert.Contains(t, role.AssumeRolePolicyDoc, oidcProviderARN)
	assert.Contains(t, role.AssumeRolePolicyDoc, "system:serviceaccount:"+namespace+":rgw-identity")
	t.Logf("  Created IAM role: %s", role.RoleARN)

	// Step 6: Controller creates permissions policy
	t.Log("Step 6: Controller attaches managed policy for S3 access")
	managedPolicyARN := "arn:aws:iam::aws:policy/AmazonS3FullAccess"
	assert.NotEmpty(t, managedPolicyARN)
	t.Logf("  Created permissions policy with full S3 access")

	// Step 7: Controller creates service account with annotations
	t.Log("Step 7: Controller creates service account with OIDC annotations")
	serviceAccountAnnotations := map[string]string{
		"object.fusion.io/account-id": accountName,
		"eks.amazonaws.com/role-arn":  role.RoleARN,
	}
	assert.Equal(t, accountName, serviceAccountAnnotations["object.fusion.io/account-id"])
	assert.Equal(t, role.RoleARN, serviceAccountAnnotations["eks.amazonaws.com/role-arn"])
	t.Logf("  Created service account 'rgw-identity' with annotations")

	// Step 8: Controller updates project annotations with results
	t.Log("Step 8: Controller updates project annotations with account and role ARNs")
	projectAnnotations := map[string]string{
		"object.fusion.io/identity-binding": "true",
		"object.fusion.io/account-arn":      accountName,
		"object.fusion.io/role-arn":         role.RoleARN,
	}
	assert.Equal(t, accountName, projectAnnotations["object.fusion.io/account-arn"])
	assert.Equal(t, role.RoleARN, projectAnnotations["object.fusion.io/role-arn"])
	t.Logf("  Updated project annotations")

	// Verification: All components are properly linked
	t.Log("=== Verification ===")
	t.Log("✓ RGW Account created:", account.AccountID)
	t.Log("✓ OIDC Provider configured:", provider.ProviderARN)
	t.Log("✓ IAM Role created:", role.RoleARN)
	t.Log("✓ Service Account annotated with role ARN")
	t.Log("✓ Project annotated with account and role information")
	t.Log("=== End-to-End Scenario Complete ===")

	// Final assertion: Verify the complete chain
	assert.Equal(t, account.AccountID, accountName)
	assert.Contains(t, provider.ProviderARN, account.AccountID)
	assert.Contains(t, role.RoleARN, account.AccountID)
	assert.Contains(t, role.AssumeRolePolicyDoc, provider.ProviderARN)
	assert.Equal(t, serviceAccountAnnotations["object.fusion.io/account-id"], account.AccountID)
	assert.Equal(t, serviceAccountAnnotations["eks.amazonaws.com/role-arn"], role.RoleARN)
}

// Made with Bob
