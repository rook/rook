/*
Copyright 2020 The Kubernetes Authors.

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

package object

import (
	"fmt"

	"github.com/aws/aws-sdk-go/service/s3"
	"k8s.io/apimachinery/pkg/util/json"
)

type action string

const (
	All                            action = "s3:*"
	AbortMultipartUpload           action = "s3:AbortMultipartUpload"
	CreateBucket                   action = "s3:CreateBucket"
	DeleteBucketPolicy             action = "s3:DeleteBucketPolicy"
	DeleteBucket                   action = "s3:DeleteBucket"
	DeleteBucketWebsite            action = "s3:DeleteBucketWebsite"
	DeleteObject                   action = "s3:DeleteObject"
	DeleteObjectVersion            action = "s3:DeleteObjectVersion"
	DeleteReplicationConfiguration action = "s3:DeleteReplicationConfiguration"
	GetAccelerateConfiguration     action = "s3:GetAccelerateConfiguration"
	GetBucketAcl                   action = "s3:GetBucketAcl"
	GetBucketCORS                  action = "s3:GetBucketCORS"
	GetBucketLocation              action = "s3:GetBucketLocation"
	GetBucketLogging               action = "s3:GetBucketLogging"
	GetBucketNotification          action = "s3:GetBucketNotification"
	GetBucketPolicy                action = "s3:GetBucketPolicy"
	GetBucketRequestPayment        action = "s3:GetBucketRequestPayment"
	GetBucketTagging               action = "s3:GetBucketTagging"
	GetBucketVersioning            action = "s3:GetBucketVersioning"
	GetBucketWebsite               action = "s3:GetBucketWebsite"
	GetLifecycleConfiguration      action = "s3:GetLifecycleConfiguration"
	GetObjectAcl                   action = "s3:GetObjectAcl"
	GetObject                      action = "s3:GetObject"
	GetObjectTorrent               action = "s3:GetObjectTorrent"
	GetObjectVersionAcl            action = "s3:GetObjectVersionAcl"
	GetObjectVersion               action = "s3:GetObjectVersion"
	GetObjectVersionTorrent        action = "s3:GetObjectVersionTorrent"
	GetReplicationConfiguration    action = "s3:GetReplicationConfiguration"
	ListAllMyBuckets               action = "s3:ListAllMyBuckets"
	ListBucketMultiPartUploads     action = "s3:ListBucketMultiPartUploads"
	ListBucket                     action = "s3:ListBucket"
	ListBucketVersions             action = "s3:ListBucketVersions"
	ListMultipartUploadParts       action = "s3:ListMultipartUploadParts"
	PutAccelerateConfiguration     action = "s3:PutAccelerateConfiguration"
	PutBucketAcl                   action = "s3:PutBucketAcl"
	PutBucketCORS                  action = "s3:PutBucketCORS"
	PutBucketLogging               action = "s3:PutBucketLogging"
	PutBucketNotification          action = "s3:PutBucketNotification"
	PutBucketPolicy                action = "s3:PutBucketPolicy"
	PutBucketRequestPayment        action = "s3:PutBucketRequestPayment"
	PutBucketTagging               action = "s3:PutBucketTagging"
	PutBucketVersioning            action = "s3:PutBucketVersioning"
	PutBucketWebsite               action = "s3:PutBucketWebsite"
	PutLifecycleConfiguration      action = "s3:PutLifecycleConfiguration"
	PutObjectAcl                   action = "s3:PutObjectAcl"
	PutObject                      action = "s3:PutObject"
	PutObjectVersionAcl            action = "s3:PutObjectVersionAcl"
	PutReplicationConfiguration    action = "s3:PutReplicationConfiguration"
	RestoreObject                  action = "s3:RestoreObject"
)

// AllowedActions is a lenient default list of actions
var AllowedActions = []action{
	DeleteObject,
	DeleteObjectVersion,
	GetBucketAcl,
	GetBucketCORS,
	GetBucketLocation,
	GetBucketLogging,
	GetBucketNotification,
	GetBucketTagging,
	GetBucketVersioning,
	GetBucketWebsite,
	GetObject,
	GetObjectAcl,
	GetObjectTorrent,
	GetObjectVersion,
	GetObjectVersionAcl,
	GetObjectVersionTorrent,
	ListAllMyBuckets,
	ListBucket,
	ListBucketMultiPartUploads,
	ListBucketVersions,
	ListMultipartUploadParts,
	PutBucketTagging,
	PutBucketVersioning,
	PutBucketWebsite,
	PutBucketVersioning,
	PutLifecycleConfiguration,
	PutObject,
	PutObjectAcl,
	PutObjectVersionAcl,
	PutReplicationConfiguration,
	RestoreObject,
}

type effect string

// effectAllow and effectDeny values are expected by the S3 API to be 'Allow' or 'Deny' explicitly
const (
	effectAllow effect = "Allow"
	effectDeny  effect = "Deny"
)

// PolicyStatement is the Go representation of a PolicyStatement json struct
// it defines what Actions that a Principle can or cannot perform on a Resource
type PolicyStatement struct {
	// Sid (optional) is the PolicyStatement's unique  identifier
	Sid string `json:"Sid"`
	// Effect determines whether the Action(s) are 'Allow'ed or 'Deny'ed.
	Effect effect `json:"Effect"`
	// Principle is/are the Ceph user names affected by this PolicyStatement
	// Must be in the format of 'arn:aws:iam:::user/<ceph-user>'
	Principal map[string][]string `json:"Principal"`
	// Action is a list of s3:* actions
	Action []action `json:"Action"`
	// Resource is the ARN identifier for the S3 resource (bucket)
	// Must be in the format of 'arn:aws:s3:::<bucket>'
	Resource []string `json:"Resource"`
}

// BucketPolicy represents set of policy statements for a single bucket.
type BucketPolicy struct {
	// Id (optional) identifies the bucket policy
	Id string `json:"Id"`
	// Version is the version of the BucketPolicy data structure
	// should always be '2012-10-17'
	Version   string            `json:"Version"`
	Statement []PolicyStatement `json:"Statement"`
}

// the version of the BucketPolicy json structure
const version = "2012-10-17"

// NewBucketPolicy obviously returns a new BucketPolicy.  PolicyStatements may be passed in at creation
// or added after the fact.  BucketPolicies should be passed to PutBucketPolicy().
func NewBucketPolicy(ps ...PolicyStatement) *BucketPolicy {
	bp := &BucketPolicy{
		Version:   version,
		Statement: append([]PolicyStatement{}, ps...),
	}
	return bp
}

// PutBucketPolicy applies the policy to the bucket
func (s *S3Agent) PutBucketPolicy(bucket string, policy BucketPolicy) (*s3.PutBucketPolicyOutput, error) {

	confirmRemoveSelfBucketAccess := false
	serializedPolicy, _ := json.Marshal(policy)
	consumablePolicy := string(serializedPolicy)

	p := &s3.PutBucketPolicyInput{
		Bucket:                        &bucket,
		ConfirmRemoveSelfBucketAccess: &confirmRemoveSelfBucketAccess,
		Policy:                        &consumablePolicy,
	}
	out, err := s.Client.PutBucketPolicy(p)
	if err != nil {
		return out, err
	}
	return out, nil
}

func (s *S3Agent) GetBucketPolicy(bucket string) (*BucketPolicy, error) {
	out, err := s.Client.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: &bucket,
	})
	if err != nil {
		return nil, err
	}

	policy := &BucketPolicy{}
	err = json.Unmarshal([]byte(*out.Policy), policy)
	if err != nil {
		return nil, err
	}
	return policy, nil
}

// ModifyBucketPolicy new and old statement SIDs and overwrites on a match.
// This allows users to Get, modify, and Replace existing statements as well as
// add new ones.
func (bp *BucketPolicy) ModifyBucketPolicy(ps ...PolicyStatement) *BucketPolicy {
	for _, newP := range ps {
		var match bool
		for j, oldP := range bp.Statement {
			if newP.Sid == oldP.Sid {
				bp.Statement[j] = newP
			}
		}
		if !match {
			bp.Statement = append(bp.Statement, newP)
		}
	}
	return bp
}

func (bp *BucketPolicy) DropPolicyStatements(sid ...string) *BucketPolicy {
	for _, s := range sid {
		for i, stmt := range bp.Statement {
			if stmt.Sid == s {
				bp.Statement = append(bp.Statement[:i], bp.Statement[i+1:]...)
				break
			}
		}
	}
	return bp
}

func (bp *BucketPolicy) EjectPrincipals(users ...string) *BucketPolicy {
	statements := bp.Statement
	for _, s := range statements {
		s.EjectPrincipals(users...)
	}
	bp.Statement = statements
	return bp
}

// NewPolicyStatement generates a new PolicyStatement. PolicyStatement methods are designed to
// be chain called with dot notation to allow for easy configuration at creation.  This is preferable
// to a long parameter list.
func NewPolicyStatement() *PolicyStatement {
	return &PolicyStatement{
		Sid:       "",
		Effect:    "",
		Principal: map[string][]string{},
		Action:    []action{},
		Resource:  []string{},
	}
}

func (ps *PolicyStatement) WithSID(sid string) *PolicyStatement {
	ps.Sid = sid
	return ps
}

const awsPrinciple = "AWS"
const arnPrefixPrinciple = "arn:aws:iam:::user/%s"
const arnPrefixResource = "arn:aws:s3:::%s"

// ForPrincipals adds users to the PolicyStatement
func (ps *PolicyStatement) ForPrincipals(users ...string) *PolicyStatement {
	principals := ps.Principal[awsPrinciple]
	for _, u := range users {
		principals = append(principals, fmt.Sprintf(arnPrefixPrinciple, u))
	}
	ps.Principal[awsPrinciple] = principals
	return ps
}

// ForResources adds resources (buckets) to the PolicyStatement with the appropriate ARN prefix
func (ps *PolicyStatement) ForResources(resources ...string) *PolicyStatement {
	for _, v := range resources {
		ps.Resource = append(ps.Resource, fmt.Sprintf(arnPrefixResource, v))
	}
	return ps
}

// ForSubResources add contents inside the bucket to the PolicyStatement with the appropriate ARN prefix
func (ps *PolicyStatement) ForSubResources(resources ...string) *PolicyStatement {
	var subresource string
	for _, v := range resources {
		subresource = fmt.Sprintf("%s/*", v)
		ps.Resource = append(ps.Resource, fmt.Sprintf(arnPrefixResource, subresource))
	}
	return ps
}

// Allows sets the effect of the PolicyStatement to allow PolicyStatement's Actions
func (ps *PolicyStatement) Allows() *PolicyStatement {
	if ps.Effect != "" {
		return ps
	}
	ps.Effect = effectAllow
	return ps
}

// Denies sets the effect of the PolicyStatement to deny the PolicyStatement's Actions
func (ps *PolicyStatement) Denies() *PolicyStatement {
	if ps.Effect != "" {
		return ps
	}
	ps.Effect = effectDeny
	return ps
}

// Actions is the set of "s3:*" actions for the PolicyStatement is concerned
func (ps *PolicyStatement) Actions(actions ...action) *PolicyStatement {
	ps.Action = actions
	return ps
}

func (ps *PolicyStatement) EjectPrincipals(users ...string) {
	principals := ps.Principal[awsPrinciple]
	for _, u := range users {
		for j, v := range principals {
			if u == v {
				principals = append(principals[:j], principals[:j+1]...)
			}
		}
	}
	ps.Principal[awsPrinciple] = principals
}

// //////////////
// End Policy
// //////////////
