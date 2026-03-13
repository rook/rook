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

// Policy represents an IAM policy within an RGW Account
type Policy struct {
	PolicyName     string `json:"policy_name"`
	PolicyDocument string `json:"policy_document"`
	PolicyARN      string `json:"policy_arn"`
}

// PolicyStatement represents a single statement in an IAM policy
type PolicyStatement struct {
	Effect    string                 `json:"Effect"`
	Action    interface{}            `json:"Action"`              // string or []string
	Resource  interface{}            `json:"Resource"`            // string or []string
	Principal interface{}            `json:"Principal,omitempty"` // for AssumeRole policies
	Condition map[string]interface{} `json:"Condition,omitempty"`
}

// PolicyDocument represents an IAM policy document
type PolicyDocument struct {
	Version   string            `json:"Version"`
	Statement []PolicyStatement `json:"Statement"`
}

// CreatePolicy creates an IAM policy within an RGW User Account
func CreatePolicy(c *object.Context, accountName, policyName, policyDocument string) (*Policy, error) {
	logger.Infof("creating IAM policy %q for account %q", policyName, accountName)

	args := []string{
		"policy",
		"create",
		"--account-name", accountName,
		"--policy-name", policyName,
		"--policy-doc", policyDocument,
	}

	result, err := object.RunAdminCommandNoMultisite(c, false, args...)
	if err != nil {
		if strings.Contains(result, "policy already exists") {
			return nil, errors.Errorf("IAM policy %q already exists for account %q", policyName, accountName)
		}
		return nil, errors.Wrapf(err, "failed to create IAM policy %q for account %q. %s", policyName, accountName, result)
	}

	// Parse the result
	var policy Policy
	err = json.Unmarshal([]byte(result), &policy)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse IAM policy creation result. %s", result)
	}

	logger.Infof("successfully created IAM policy %q for account %q", policyName, accountName)
	return &policy, nil
}

// AttachRolePolicy attaches a policy to a role within an RGW User Account
func AttachRolePolicy(c *object.Context, accountName, roleName, policyARN string) error {
	logger.Infof("attaching policy %q to role %q in account %q", policyARN, roleName, accountName)

	args := []string{
		"role",
		"attach-policy",
		"--account-name", accountName,
		"--role-name", roleName,
		"--policy-arn", policyARN,
	}

	result, err := object.RunAdminCommandNoMultisite(c, false, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to attach policy %q to role %q. %s", policyARN, roleName, result)
	}

	logger.Infof("successfully attached policy %q to role %q", policyARN, roleName)
	return nil
}

// DetachRolePolicy detaches a policy from a role within an RGW User Account
func DetachRolePolicy(c *object.Context, accountName, roleName, policyARN string) error {
	logger.Infof("detaching policy %q from role %q in account %q", policyARN, roleName, accountName)

	args := []string{
		"role",
		"detach-policy",
		"--account-name", accountName,
		"--role-name", roleName,
		"--policy-arn", policyARN,
	}

	result, err := object.RunAdminCommandNoMultisite(c, false, args...)
	if err != nil {
		return errors.Wrapf(err, "failed to detach policy %q from role %q. %s", policyARN, roleName, result)
	}

	logger.Infof("successfully detached policy %q from role %q", policyARN, roleName)
	return nil
}

// DeletePolicy deletes an IAM policy from an RGW User Account
func DeletePolicy(c *object.Context, accountName, policyARN string) error {
	logger.Infof("deleting IAM policy %q from account %q", policyARN, accountName)

	args := []string{
		"policy",
		"delete",
		"--account-name", accountName,
		"--policy-arn", policyARN,
	}

	result, err := object.RunAdminCommandNoMultisite(c, false, args...)
	if err != nil {
		if strings.Contains(result, "policy not found") {
			logger.Infof("IAM policy %q not found, already deleted", policyARN)
			return nil
		}
		return errors.Wrapf(err, "failed to delete IAM policy %q. %s", policyARN, result)
	}

	logger.Infof("successfully deleted IAM policy %q", policyARN)
	return nil
}

// GenerateFullAccessPolicy generates a policy document for full S3 access within the account
func GenerateFullAccessPolicy(accountName string) string {
	doc := PolicyDocument{
		Version: "2012-10-17",
		Statement: []PolicyStatement{
			{
				Effect:   "Allow",
				Action:   "s3:*",
				Resource: "*",
			},
		},
	}

	jsonBytes, _ := json.MarshalIndent(doc, "", "  ")
	return string(jsonBytes)
}

// GenerateReadOnlyPolicy generates a policy document for read-only S3 access
func GenerateReadOnlyPolicy(accountName string, bucketPrefix string) string {
	resource := "*"
	if bucketPrefix != "" {
		resource = fmt.Sprintf("arn:aws:s3:::%s*", bucketPrefix)
	}

	doc := PolicyDocument{
		Version: "2012-10-17",
		Statement: []PolicyStatement{
			{
				Effect: "Allow",
				Action: []string{
					"s3:GetObject",
					"s3:GetObjectVersion",
					"s3:ListBucket",
					"s3:ListBucketVersions",
				},
				Resource: resource,
			},
		},
	}

	jsonBytes, _ := json.MarshalIndent(doc, "", "  ")
	return string(jsonBytes)
}

// GenerateWriteOnlyPolicy generates a policy document for write-only S3 access
func GenerateWriteOnlyPolicy(accountName string, bucketPrefix string) string {
	resource := "*"
	if bucketPrefix != "" {
		resource = fmt.Sprintf("arn:aws:s3:::%s*", bucketPrefix)
	}

	doc := PolicyDocument{
		Version: "2012-10-17",
		Statement: []PolicyStatement{
			{
				Effect: "Allow",
				Action: []string{
					"s3:PutObject",
					"s3:PutObjectAcl",
					"s3:DeleteObject",
				},
				Resource: resource,
			},
		},
	}

	jsonBytes, _ := json.MarshalIndent(doc, "", "  ")
	return string(jsonBytes)
}

// GenerateBucketSpecificPolicy generates a policy for specific bucket access
func GenerateBucketSpecificPolicy(accountName string, bucketName string, allowedActions []string) string {
	if len(allowedActions) == 0 {
		allowedActions = []string{"s3:*"}
	}

	doc := PolicyDocument{
		Version: "2012-10-17",
		Statement: []PolicyStatement{
			{
				Effect: "Allow",
				Action: allowedActions,
				Resource: []string{
					fmt.Sprintf("arn:aws:s3:::%s", bucketName),
					fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
				},
			},
		},
	}

	jsonBytes, _ := json.MarshalIndent(doc, "", "  ")
	return string(jsonBytes)
}

// GeneratePathBasedPolicy generates a policy for path-based access within buckets
func GeneratePathBasedPolicy(accountName string, bucketName string, pathPrefix string, allowedActions []string) string {
	if len(allowedActions) == 0 {
		allowedActions = []string{"s3:GetObject", "s3:PutObject"}
	}

	doc := PolicyDocument{
		Version: "2012-10-17",
		Statement: []PolicyStatement{
			{
				Effect:   "Allow",
				Action:   allowedActions,
				Resource: fmt.Sprintf("arn:aws:s3:::%s/%s*", bucketName, pathPrefix),
			},
			{
				Effect:   "Allow",
				Action:   "s3:ListBucket",
				Resource: fmt.Sprintf("arn:aws:s3:::%s", bucketName),
				Condition: map[string]interface{}{
					"StringLike": map[string]string{
						"s3:prefix": fmt.Sprintf("%s*", pathPrefix),
					},
				},
			},
		},
	}

	jsonBytes, _ := json.MarshalIndent(doc, "", "  ")
	return string(jsonBytes)
}

// GenerateQuotaEnforcedPolicy generates a policy with quota enforcement
func GenerateQuotaEnforcedPolicy(accountName string, maxSizeBytes int64) string {
	doc := PolicyDocument{
		Version: "2012-10-17",
		Statement: []PolicyStatement{
			{
				Effect:   "Allow",
				Action:   "s3:*",
				Resource: "*",
				Condition: map[string]interface{}{
					"NumericLessThanEquals": map[string]int64{
						"s3:content-length": maxSizeBytes,
					},
				},
			},
		},
	}

	jsonBytes, _ := json.MarshalIndent(doc, "", "  ")
	return string(jsonBytes)
}

// Made with Bob
