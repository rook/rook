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
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/ceph/object"
)

// RGWAccount represents a Ceph RGW User Account (Ceph 8.1+)
type RGWAccount struct {
	AccountID   string `json:"account_id"`
	AccountName string `json:"account_name"`
	Email       string `json:"email,omitempty"`
	// Additional fields as needed
}

// OIDCProvider represents an OIDC provider configuration within an RGW Account
type OIDCProvider struct {
	ProviderARN string   `json:"provider_arn"`
	Issuer      string   `json:"issuer"`
	Thumbprints []string `json:"thumbprints"`
}

// IAMRole represents an IAM role within an RGW Account
type IAMRole struct {
	RoleARN              string `json:"role_arn"`
	RoleName             string `json:"role_name"`
	AssumeRolePolicyDoc  string `json:"assume_role_policy_document"`
	PermissionsPolicyDoc string `json:"permissions_policy_document,omitempty"`
}

// CreateAccount creates a new RGW User Account
func CreateAccount(c *object.Context, accountName string) (*RGWAccount, error) {
	logger.Infof("creating RGW User Account %q", accountName)

	args := []string{
		"account",
		"create",
		"--account-name", accountName,
	}

	result, err := object.RunAdminCommandNoMultisite(c, false, args...)
	if err != nil {
		if strings.Contains(result, "account already exists") {
			return nil, errors.Errorf("RGW User Account %q already exists", accountName)
		}
		return nil, errors.Wrapf(err, "failed to create RGW User Account %q. %s", accountName, result)
	}

	// Parse the result
	var account RGWAccount
	err = json.Unmarshal([]byte(result), &account)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse RGW User Account creation result. %s", result)
	}

	logger.Infof("successfully created RGW User Account %q", accountName)
	return &account, nil
}

// GetAccount retrieves information about an RGW User Account
func GetAccount(c *object.Context, accountName string) (*RGWAccount, error) {
	logger.Debugf("getting RGW User Account %q", accountName)

	args := []string{
		"account",
		"info",
		"--account-name", accountName,
	}

	result, err := object.RunAdminCommandNoMultisite(c, false, args...)
	if err != nil {
		if strings.Contains(result, "account not found") || strings.Contains(result, "no account info saved") {
			return nil, errors.Errorf("RGW User Account %q not found", accountName)
		}
		return nil, errors.Wrapf(err, "failed to get RGW User Account %q. %s", accountName, result)
	}

	// Parse the result
	var account RGWAccount
	err = json.Unmarshal([]byte(result), &account)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse RGW User Account info. %s", result)
	}

	return &account, nil
}

// DeleteAccount deletes an RGW User Account
func DeleteAccount(c *object.Context, accountName string) error {
	logger.Infof("deleting RGW User Account %q", accountName)

	args := []string{
		"account",
		"rm",
		"--account-name", accountName,
	}

	result, err := object.RunAdminCommandNoMultisite(c, false, args...)
	if err != nil {
		if strings.Contains(result, "account not found") {
			logger.Infof("RGW User Account %q not found, already deleted", accountName)
			return nil
		}
		return errors.Wrapf(err, "failed to delete RGW User Account %q. %s", accountName, result)
	}

	logger.Infof("successfully deleted RGW User Account %q", accountName)
	return nil
}

// CreateOIDCProvider creates an OIDC provider within an RGW User Account
func CreateOIDCProvider(c *object.Context, accountName, issuer string, thumbprints []string) (*OIDCProvider, error) {
	logger.Infof("creating OIDC provider for account %q with issuer %q", accountName, issuer)

	args := []string{
		"oidc-provider",
		"create",
		"--account-name", accountName,
		"--issuer", issuer,
	}

	// Add thumbprints
	for _, thumbprint := range thumbprints {
		args = append(args, "--thumbprint", thumbprint)
	}

	result, err := object.RunAdminCommandNoMultisite(c, false, args...)
	if err != nil {
		if strings.Contains(result, "provider already exists") {
			return nil, errors.Errorf("OIDC provider already exists for account %q", accountName)
		}
		return nil, errors.Wrapf(err, "failed to create OIDC provider for account %q. %s", accountName, result)
	}

	// Parse the result
	var provider OIDCProvider
	err = json.Unmarshal([]byte(result), &provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse OIDC provider creation result. %s", result)
	}

	logger.Infof("successfully created OIDC provider for account %q", accountName)
	return &provider, nil
}

// CreateRole creates an IAM role within an RGW User Account
func CreateRole(c *object.Context, accountName, roleName, assumeRolePolicyDoc string) (*IAMRole, error) {
	logger.Infof("creating IAM role %q for account %q", roleName, accountName)

	args := []string{
		"role",
		"create",
		"--account-name", accountName,
		"--role-name", roleName,
		"--assume-role-policy-doc", assumeRolePolicyDoc,
	}

	result, err := object.RunAdminCommandNoMultisite(c, false, args...)
	if err != nil {
		if strings.Contains(result, "role already exists") {
			return nil, errors.Errorf("IAM role %q already exists for account %q", roleName, accountName)
		}
		return nil, errors.Wrapf(err, "failed to create IAM role %q for account %q. %s", roleName, accountName, result)
	}

	// Parse the result
	var role IAMRole
	err = json.Unmarshal([]byte(result), &role)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse IAM role creation result. %s", result)
	}

	logger.Infof("successfully created IAM role %q for account %q", roleName, accountName)
	return &role, nil
}

// GenerateAssumeRolePolicyDocument generates an assume role policy document for OIDC federation
func GenerateAssumeRolePolicyDocument(accountName, oidcProviderARN, namespace string) string {
	// This policy allows the service account in the namespace to assume the role
	// using AssumeRoleWithWebIdentity
	policy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "%s"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "oidc:sub": "system:serviceaccount:%s:rgw-identity"
        }
      }
    }
  ]
}`, oidcProviderARN, namespace)

	return policy
}

// GeneratePermissionsPolicyDocument generates a permissions policy document for the role
func GeneratePermissionsPolicyDocument(accountName string) string {
	// This policy grants full S3 access within the account
	policy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "s3:*",
      "Resource": [
        "arn:aws:s3:::*"
      ]
    }
  ]
}`)

	return policy
}

// Made with Bob
