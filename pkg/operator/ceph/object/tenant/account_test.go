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

func TestGenerateAssumeRolePolicyDocument(t *testing.T) {
	accountName := "test-account"
	oidcProviderARN := "arn:aws:iam::test-account:oidc-provider/kubernetes.default.svc"
	namespace := "test-namespace"

	policy := GenerateAssumeRolePolicyDocument(accountName, oidcProviderARN, namespace)

	// Verify it's valid JSON
	var doc PolicyDocument
	err := json.Unmarshal([]byte(policy), &doc)
	assert.NoError(t, err)

	// Verify structure
	assert.Equal(t, "2012-10-17", doc.Version)
	assert.Len(t, doc.Statement, 1)

	stmt := doc.Statement[0]
	assert.Equal(t, "Allow", stmt.Effect)
	assert.Equal(t, "sts:AssumeRoleWithWebIdentity", stmt.Action)

	// Verify principal
	principal, ok := stmt.Principal.(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, principal, "Federated")
	assert.Equal(t, oidcProviderARN, principal["Federated"])

	// Verify condition
	assert.NotNil(t, stmt.Condition)
	condition := stmt.Condition
	assert.Contains(t, condition, "StringEquals")

	stringEquals, ok := condition["StringEquals"].(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, stringEquals, "oidc:sub")
	assert.Equal(t, "system:serviceaccount:test-namespace:rgw-identity", stringEquals["oidc:sub"])
}

func TestRGWAccountStructure(t *testing.T) {
	// Test that RGWAccount can be marshaled/unmarshaled
	account := RGWAccount{
		AccountID:   "RGW123456",
		AccountName: "test-account",
		Email:       "test@example.com",
	}

	jsonBytes, err := json.Marshal(account)
	assert.NoError(t, err)

	var account2 RGWAccount
	err = json.Unmarshal(jsonBytes, &account2)
	assert.NoError(t, err)

	assert.Equal(t, account.AccountID, account2.AccountID)
	assert.Equal(t, account.AccountName, account2.AccountName)
	assert.Equal(t, account.Email, account2.Email)
}

func TestOIDCProviderStructure(t *testing.T) {
	// Test that OIDCProvider can be marshaled/unmarshaled
	provider := OIDCProvider{
		ProviderARN: "arn:aws:iam::account:oidc-provider/issuer",
		Issuer:      "https://kubernetes.default.svc",
		Thumbprints: []string{"ABCD1234", "EFGH5678"},
	}

	jsonBytes, err := json.Marshal(provider)
	assert.NoError(t, err)

	var provider2 OIDCProvider
	err = json.Unmarshal(jsonBytes, &provider2)
	assert.NoError(t, err)

	assert.Equal(t, provider.ProviderARN, provider2.ProviderARN)
	assert.Equal(t, provider.Issuer, provider2.Issuer)
	assert.Equal(t, provider.Thumbprints, provider2.Thumbprints)
}

func TestIAMRoleStructure(t *testing.T) {
	// Test that IAMRole can be marshaled/unmarshaled
	role := IAMRole{
		RoleARN:              "arn:aws:iam::account:role/test-role",
		RoleName:             "test-role",
		AssumeRolePolicyDoc:  `{"Version":"2012-10-17"}`,
		PermissionsPolicyDoc: `{"Version":"2012-10-17"}`,
	}

	jsonBytes, err := json.Marshal(role)
	assert.NoError(t, err)

	var role2 IAMRole
	err = json.Unmarshal(jsonBytes, &role2)
	assert.NoError(t, err)

	assert.Equal(t, role.RoleARN, role2.RoleARN)
	assert.Equal(t, role.RoleName, role2.RoleName)
	assert.Equal(t, role.AssumeRolePolicyDoc, role2.AssumeRolePolicyDoc)
	assert.Equal(t, role.PermissionsPolicyDoc, role2.PermissionsPolicyDoc)
}

func TestPolicyStatementWithPrincipal(t *testing.T) {
	// Test PolicyStatement with Principal field
	stmt := PolicyStatement{
		Effect: "Allow",
		Action: "sts:AssumeRoleWithWebIdentity",
		Principal: map[string]interface{}{
			"Federated": "arn:aws:iam::account:oidc-provider/issuer",
		},
		Condition: map[string]interface{}{
			"StringEquals": map[string]string{
				"oidc:sub": "system:serviceaccount:ns:sa",
			},
		},
	}

	jsonBytes, err := json.Marshal(stmt)
	assert.NoError(t, err)

	var stmt2 PolicyStatement
	err = json.Unmarshal(jsonBytes, &stmt2)
	assert.NoError(t, err)

	assert.Equal(t, stmt.Effect, stmt2.Effect)
	assert.Equal(t, stmt.Action, stmt2.Action)
	assert.NotNil(t, stmt2.Principal)
	assert.NotNil(t, stmt2.Condition)
}

// Made with Bob
