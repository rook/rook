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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/ceph/object"
)

// RGWAccount represents a Ceph RGW User Account (Ceph 8.1+)
type RGWAccount struct {
	AccountID   string `json:"id"`
	Tenant      string `json:"tenant,omitempty"`
	AccountName string `json:"name"`
	Email       string `json:"email,omitempty"`
	// Additional fields as needed
}

// AccountRootUser represents the root user credentials for an RGW Account
type AccountRootUser struct {
	UserID    string `json:"user_id"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
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
	logger.Infof("CreateAccount: starting for account %q", accountName)
	logger.Infof("CreateAccount: Context - Name=%q, Realm=%q, Zone=%q, ZoneGroup=%q", c.Name, c.Realm, c.Zone, c.ZoneGroup)

	args := []string{
		"account",
		"create",
		"--account-name", accountName,
	}

	logger.Infof("CreateAccount: calling radosgw-admin with args: %v", args)
	logger.Infof("CreateAccount: about to call object.RunAdminCommand")
	result, err := object.RunAdminCommand(c, false, args...)
	logger.Infof("CreateAccount: radosgw-admin returned, err=%v, result length=%d", err, len(result))

	if err != nil {
		// Check if account already exists - radosgw-admin returns "File exists" error
		if strings.Contains(result, "File exists") || strings.Contains(result, "account already exists") {
			logger.Infof("CreateAccount: account %q already exists, retrieving existing account", accountName)
			return GetAccount(c, accountName)
		}
		logger.Errorf("CreateAccount: failed to create account %q: %v, result: %s", accountName, err, result)
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
	logger.Infof("GetAccount: retrieving RGW User Account %q", accountName)

	args := []string{
		"account",
		"get",
		"--account-name", accountName,
	}

	logger.Infof("GetAccount: calling radosgw-admin with args: %v", args)
	result, err := object.RunAdminCommand(c, false, args...)
	logger.Infof("GetAccount: radosgw-admin returned, err=%v, result length=%d", err, len(result))

	if err != nil {
		if strings.Contains(result, "account not found") || strings.Contains(result, "no account info saved") {
			logger.Errorf("GetAccount: account %q not found", accountName)
			return nil, errors.Errorf("RGW User Account %q not found", accountName)
		}
		logger.Errorf("GetAccount: failed to get account %q: %v, result: %s", accountName, err, result)
		return nil, errors.Wrapf(err, "failed to get RGW User Account %q. %s", accountName, result)
	}

	// Parse the result
	logger.Infof("GetAccount: parsing JSON result for account %q", accountName)
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

	result, err := object.RunAdminCommand(c, false, args...)
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

// CreateAccountRootUser creates a root user for an RGW Account
// The root user has full permissions within the account and can manage IAM resources
func CreateAccountRootUser(c *object.Context, accountID string, accountName string) (*AccountRootUser, error) {
	logger.Infof("creating root user for account %q (name: %q)", accountID, accountName)

	// Use account ID as the root user ID since it's guaranteed to be alphanumeric
	// RGW account root users have strict validation - only alphanumeric characters allowed
	// Account IDs are in format RGWxxxxxxxxxxxxxxxxx which is safe
	userID := fmt.Sprintf("%s-root", accountID)
	logger.Infof("creating root user with userID: %q for account %q", userID, accountID)

	// Display name cannot contain spaces for account root users
	displayName := fmt.Sprintf("RootUserFor%s", accountID)

	args := []string{
		"user",
		"create",
		"--uid", userID,
		"--display-name", displayName,
		"--account-id", accountID,
		"--account-root",
	}

	result, err := object.RunAdminCommand(c, false, args...)
	if err != nil {
		// Check if user already exists
		if strings.Contains(result, "could not create user") && strings.Contains(result, "exists") {
			logger.Infof("root user %q already exists for account %q, retrieving info", userID, accountID)
			return GetAccountRootUser(c, userID)
		}
		logger.Errorf("failed to create root user for account %q: %v, result: %s", accountID, err, result)
		return nil, errors.Wrapf(err, "failed to create root user for account %q. %s", accountID, result)
	}

	// Add OIDC provider capability to the root user
	// This is required for the user to manage OIDC providers via the IAM API
	capsArgs := []string{
		"caps",
		"add",
		"--uid", userID,
		"--caps", "oidc-provider=*",
	}
	_, capsErr := object.RunAdminCommand(c, false, capsArgs...)
	if capsErr != nil {
		logger.Warningf("failed to add oidc-provider capability to root user %q: %v", userID, capsErr)
		// Don't fail - the user was created, just without the capability
	} else {
		logger.Infof("added oidc-provider capability to root user %q", userID)
	}

	// Parse the result to extract access key and secret key
	var userInfo struct {
		UserID string `json:"user_id"`
		Keys   []struct {
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
		} `json:"keys"`
	}

	err = json.Unmarshal([]byte(result), &userInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse root user creation result. %s", result)
	}

	if len(userInfo.Keys) == 0 {
		return nil, errors.Errorf("no keys generated for root user %q", userID)
	}

	rootUser := &AccountRootUser{
		UserID:    userInfo.UserID,
		AccessKey: userInfo.Keys[0].AccessKey,
		SecretKey: userInfo.Keys[0].SecretKey,
	}

	logger.Infof("successfully created root user %q for account %q", userID, accountID)
	return rootUser, nil
}

// GetAccountRootUser retrieves the root user information for an account
func GetAccountRootUser(c *object.Context, userID string) (*AccountRootUser, error) {
	logger.Infof("retrieving root user %q", userID)

	args := []string{
		"user",
		"info",
		"--uid", userID,
	}

	result, err := object.RunAdminCommand(c, false, args...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get root user %q. %s", userID, result)
	}

	// Parse the result
	var userInfo struct {
		UserID string `json:"user_id"`
		Keys   []struct {
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
		} `json:"keys"`
	}

	err = json.Unmarshal([]byte(result), &userInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse root user info. %s", result)
	}

	if len(userInfo.Keys) == 0 {
		return nil, errors.Errorf("no keys found for root user %q", userID)
	}

	// Ensure the root user has OIDC provider capability
	// This is needed even for existing users that may have been created without it
	capsArgs := []string{
		"caps",
		"add",
		"--uid", userID,
		"--caps", "oidc-provider=*",
	}
	_, capsErr := object.RunAdminCommand(c, false, capsArgs...)
	if capsErr != nil {
		logger.Warningf("failed to add oidc-provider capability to existing root user %q: %v", userID, capsErr)
		// Don't fail - the user exists, just may not have the capability
	} else {
		logger.Infof("ensured oidc-provider capability on existing root user %q", userID)
	}

	return &AccountRootUser{
		UserID:    userInfo.UserID,
		AccessKey: userInfo.Keys[0].AccessKey,
		SecretKey: userInfo.Keys[0].SecretKey,
	}, nil
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

	result, err := object.RunAdminCommand(c, false, args...)
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

// CreateRole creates an IAM role within an RGW User Account using radosgw-admin
// Deprecated: Use CreateRoleViaAPI for new code
func CreateRole(c *object.Context, accountName, roleName, assumeRolePolicyDoc string) (*IAMRole, error) {
	logger.Infof("creating IAM role %q for account %q", roleName, accountName)

	args := []string{
		"role",
		"create",
		"--account-id", accountName,
		"--role-name", roleName,
		"--assume-role-policy-doc", assumeRolePolicyDoc,
	}

	result, err := object.RunAdminCommand(c, false, args...)
	if err != nil {
		if strings.Contains(result, "role already exists") {
			logger.Infof("IAM role %q already exists for account %q, continuing", roleName, accountName)
			// Try to get the existing role
			return GetRole(c, accountName, roleName)
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

// CreateRoleViaAPI creates an IAM role using the IAM API
func CreateRoleViaAPI(iamClient *iam.IAM, roleName, assumeRolePolicyDoc string) (*IAMRole, error) {
	logger.Infof("creating IAM role %q via IAM API", roleName)

	input := &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(assumeRolePolicyDoc),
	}

	output, err := iamClient.CreateRole(input)
	if err != nil {
		// Check if role already exists
		if strings.Contains(err.Error(), "EntityAlreadyExists") {
			logger.Infof("IAM role %q already exists, continuing", roleName)
			// Try to get the existing role
			return GetRoleViaAPI(iamClient, roleName)
		}
		return nil, errors.Wrapf(err, "failed to create IAM role %q via IAM API", roleName)
	}

	role := &IAMRole{
		RoleARN:              aws.StringValue(output.Role.Arn),
		RoleName:             aws.StringValue(output.Role.RoleName),
		AssumeRolePolicyDoc:  aws.StringValue(output.Role.AssumeRolePolicyDocument),
		PermissionsPolicyDoc: "",
	}

	logger.Infof("successfully created IAM role %q via IAM API with ARN %q", roleName, role.RoleARN)
	return role, nil
}

// GetRoleViaAPI retrieves an IAM role using the IAM API
func GetRoleViaAPI(iamClient *iam.IAM, roleName string) (*IAMRole, error) {
	logger.Debugf("getting IAM role %q via IAM API", roleName)

	input := &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	}

	output, err := iamClient.GetRole(input)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get IAM role %q via IAM API", roleName)
	}

	role := &IAMRole{
		RoleARN:              aws.StringValue(output.Role.Arn),
		RoleName:             aws.StringValue(output.Role.RoleName),
		AssumeRolePolicyDoc:  aws.StringValue(output.Role.AssumeRolePolicyDocument),
		PermissionsPolicyDoc: "",
	}

	return role, nil
}

// GetRole retrieves an IAM role from an RGW User Account
func GetRole(c *object.Context, accountName, roleName string) (*IAMRole, error) {
	logger.Debugf("getting IAM role %q for account %q", roleName, accountName)

	args := []string{
		"role",
		"get",
		"--account-id", accountName,
		"--role-name", roleName,
	}

	result, err := object.RunAdminCommand(c, false, args...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get IAM role %q for account %q. %s", roleName, accountName, result)
	}

	// Parse the result
	var role IAMRole
	err = json.Unmarshal([]byte(result), &role)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse IAM role info. %s", result)
	}

	return &role, nil
}

// GenerateAssumeRolePolicyDocument generates an assume role policy document for OIDC federation
func GenerateAssumeRolePolicyDocument(accountName, oidcProviderARN, namespace string) string {
	// Extract the issuer hostname from the provider ARN
	// ARN format: arn:aws:iam::<account>:oidc-provider/<issuer>
	// Example: arn:aws:iam::RGW123:oidc-provider/kubernetes.default.svc
	issuer := "kubernetes.default.svc" // default
	parts := strings.Split(oidcProviderARN, ":oidc-provider/")
	if len(parts) == 2 {
		issuer = parts[1]
	}

	// Ceph RGW constructs condition keys as: <issuer>:<claim-name>
	// See: ceph/src/rgw/rgw_auth.cc WebIdentityApplier::modify_request_state()
	// The issuer is used without the https:// prefix
	conditionKey := issuer + ":sub"

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
          "%s": "system:serviceaccount:%s:rgw-identity"
        }
      }
    }
  ]
}`, oidcProviderARN, conditionKey, namespace)

	return policy
}

// Made with Bob
