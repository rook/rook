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

func TestGenerateFullAccessPolicy(t *testing.T) {
	accountName := "test-account"
	policy := GenerateFullAccessPolicy(accountName)

	// Verify it's valid JSON
	var doc PolicyDocument
	err := json.Unmarshal([]byte(policy), &doc)
	assert.NoError(t, err)

	// Verify structure
	assert.Equal(t, "2012-10-17", doc.Version)
	assert.Len(t, doc.Statement, 1)
	assert.Equal(t, "Allow", doc.Statement[0].Effect)
	assert.Equal(t, "s3:*", doc.Statement[0].Action)
	assert.Equal(t, "*", doc.Statement[0].Resource)
}

func TestGenerateReadOnlyPolicy(t *testing.T) {
	tests := []struct {
		name         string
		accountName  string
		bucketPrefix string
		wantResource string
	}{
		{
			name:         "no prefix",
			accountName:  "test-account",
			bucketPrefix: "",
			wantResource: "*",
		},
		{
			name:         "with prefix",
			accountName:  "test-account",
			bucketPrefix: "data-",
			wantResource: "arn:aws:s3:::data-*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := GenerateReadOnlyPolicy(tt.accountName, tt.bucketPrefix)

			var doc PolicyDocument
			err := json.Unmarshal([]byte(policy), &doc)
			assert.NoError(t, err)

			assert.Equal(t, "2012-10-17", doc.Version)
			assert.Len(t, doc.Statement, 1)
			assert.Equal(t, "Allow", doc.Statement[0].Effect)
			assert.Equal(t, tt.wantResource, doc.Statement[0].Resource)

			// Verify read-only actions
			actions, ok := doc.Statement[0].Action.([]interface{})
			assert.True(t, ok)
			assert.Contains(t, actions, "s3:GetObject")
			assert.Contains(t, actions, "s3:ListBucket")
		})
	}
}

func TestGenerateWriteOnlyPolicy(t *testing.T) {
	accountName := "test-account"
	bucketPrefix := "uploads-"
	policy := GenerateWriteOnlyPolicy(accountName, bucketPrefix)

	var doc PolicyDocument
	err := json.Unmarshal([]byte(policy), &doc)
	assert.NoError(t, err)

	assert.Equal(t, "2012-10-17", doc.Version)
	assert.Len(t, doc.Statement, 1)
	assert.Equal(t, "Allow", doc.Statement[0].Effect)
	assert.Equal(t, "arn:aws:s3:::uploads-*", doc.Statement[0].Resource)

	// Verify write-only actions
	actions, ok := doc.Statement[0].Action.([]interface{})
	assert.True(t, ok)
	assert.Contains(t, actions, "s3:PutObject")
	assert.Contains(t, actions, "s3:DeleteObject")
	assert.NotContains(t, actions, "s3:GetObject")
}

func TestGenerateBucketSpecificPolicy(t *testing.T) {
	tests := []struct {
		name           string
		accountName    string
		bucketName     string
		allowedActions []string
		wantActions    interface{}
	}{
		{
			name:           "default actions",
			accountName:    "test-account",
			bucketName:     "my-bucket",
			allowedActions: nil,
			wantActions:    []interface{}{"s3:*"},
		},
		{
			name:        "custom actions",
			accountName: "test-account",
			bucketName:  "my-bucket",
			allowedActions: []string{
				"s3:GetObject",
				"s3:PutObject",
			},
			wantActions: []interface{}{"s3:GetObject", "s3:PutObject"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := GenerateBucketSpecificPolicy(tt.accountName, tt.bucketName, tt.allowedActions)

			var doc PolicyDocument
			err := json.Unmarshal([]byte(policy), &doc)
			assert.NoError(t, err)

			assert.Equal(t, "2012-10-17", doc.Version)
			assert.Len(t, doc.Statement, 1)
			assert.Equal(t, "Allow", doc.Statement[0].Effect)

			// Verify resources include bucket and bucket/*
			resources, ok := doc.Statement[0].Resource.([]interface{})
			assert.True(t, ok)
			assert.Contains(t, resources, "arn:aws:s3:::my-bucket")
			assert.Contains(t, resources, "arn:aws:s3:::my-bucket/*")
		})
	}
}

func TestGeneratePathBasedPolicy(t *testing.T) {
	accountName := "test-account"
	bucketName := "shared-bucket"
	pathPrefix := "team-a/"
	actions := []string{"s3:GetObject", "s3:PutObject"}

	policy := GeneratePathBasedPolicy(accountName, bucketName, pathPrefix, actions)

	var doc PolicyDocument
	err := json.Unmarshal([]byte(policy), &doc)
	assert.NoError(t, err)

	assert.Equal(t, "2012-10-17", doc.Version)
	assert.Len(t, doc.Statement, 2)

	// First statement: object operations
	assert.Equal(t, "Allow", doc.Statement[0].Effect)
	assert.Equal(t, "arn:aws:s3:::shared-bucket/team-a/*", doc.Statement[0].Resource)

	// Second statement: list bucket with prefix condition
	assert.Equal(t, "Allow", doc.Statement[1].Effect)
	assert.Equal(t, "s3:ListBucket", doc.Statement[1].Action)
	assert.Equal(t, "arn:aws:s3:::shared-bucket", doc.Statement[1].Resource)
	assert.NotNil(t, doc.Statement[1].Condition)
}

func TestGenerateQuotaEnforcedPolicy(t *testing.T) {
	accountName := "test-account"
	maxSizeBytes := int64(1024 * 1024 * 100) // 100 MB

	policy := GenerateQuotaEnforcedPolicy(accountName, maxSizeBytes)

	var doc PolicyDocument
	err := json.Unmarshal([]byte(policy), &doc)
	assert.NoError(t, err)

	assert.Equal(t, "2012-10-17", doc.Version)
	assert.Len(t, doc.Statement, 1)
	assert.Equal(t, "Allow", doc.Statement[0].Effect)
	assert.Equal(t, "s3:*", doc.Statement[0].Action)
	assert.Equal(t, "*", doc.Statement[0].Resource)

	// Verify quota condition
	assert.NotNil(t, doc.Statement[0].Condition)
	condition := doc.Statement[0].Condition
	assert.Contains(t, condition, "NumericLessThanEquals")
}

func TestPolicyDocumentMarshaling(t *testing.T) {
	// Test that we can marshal and unmarshal policy documents
	doc := PolicyDocument{
		Version: "2012-10-17",
		Statement: []PolicyStatement{
			{
				Effect:   "Allow",
				Action:   "s3:GetObject",
				Resource: "arn:aws:s3:::bucket/*",
			},
			{
				Effect:   "Allow",
				Action:   []string{"s3:ListBucket", "s3:GetBucketLocation"},
				Resource: []string{"arn:aws:s3:::bucket", "arn:aws:s3:::bucket/*"},
				Condition: map[string]interface{}{
					"StringEquals": map[string]string{
						"s3:prefix": "data/",
					},
				},
			},
		},
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(doc)
	assert.NoError(t, err)

	// Unmarshal back
	var doc2 PolicyDocument
	err = json.Unmarshal(jsonBytes, &doc2)
	assert.NoError(t, err)

	// Verify structure is preserved
	assert.Equal(t, doc.Version, doc2.Version)
	assert.Len(t, doc2.Statement, 2)
}

// Made with Bob
