// Copyright 2020 IBM Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kp

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

const (
	// DualAuthDelete defines the policy type as dual auth delete
	DualAuthDelete = "dualAuthDelete"

	//RotationPolicy defines the policy type as rotation
	RotationPolicy = "rotation"

	policyType = "application/vnd.ibm.kms.policy+json"
)

// Policy represents a policy as returned by the KP API.
type Policy struct {
	Type      string     `json:"type,omitempty"`
	CreatedBy string     `json:"createdBy,omitempty"`
	CreatedAt *time.Time `json:"creationDate,omitempty"`
	CRN       string     `json:"crn,omitempty"`
	UpdatedAt *time.Time `json:"lastUpdateDate,omitempty"`
	UpdatedBy string     `json:"updatedBy,omitempty"`
	Rotation  *Rotation  `json:"rotation,omitempty"`
	DualAuth  *DualAuth  `json:"dualAuthDelete,omitempty"`
}

type Rotation struct {
	Enabled  *bool `json:"enabled,omitempty"`
	Interval int   `json:"interval_month,omitempty"`
}

type DualAuth struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// PoliciesMetadata represents the metadata of a collection of keys.
type PoliciesMetadata struct {
	CollectionType   string `json:"collectionType"`
	NumberOfPolicies int    `json:"collectionTotal"`
}

// Policies represents a collection of Policies.
type Policies struct {
	Metadata PoliciesMetadata `json:"metadata"`
	Policies []Policy         `json:"resources"`
}

// GetPolicy retrieves a policy by Key ID or alias. This function is
// deprecated, as it only returns one policy and does not let you
// select which policy set it will return. It is kept for backward
// compatibility on keys with only one rotation policy. Please update
// to use the new GetPolicies or Get<type>Policy functions.
func (c *Client) GetPolicy(ctx context.Context, idOrAlias string) (*Policy, error) {
	policyresponse := Policies{}

	req, err := c.newRequest("GET", fmt.Sprintf("keys/%s/policies", idOrAlias), nil)
	if err != nil {
		return nil, err
	}

	_, err = c.do(ctx, req, &policyresponse)
	if err != nil {
		return nil, err
	}

	return &policyresponse.Policies[0], nil
}

// SetPolicy updates a policy resource by specifying the ID of the key and
// the rotation interval needed. This function is deprecated as it will only
// let you set key rotation  policies. To set dual auth and other newer policies
// on a key, please use the new SetPolicies of Set<type>Policy functions.
func (c *Client) SetPolicy(ctx context.Context, idOrAlias string, prefer PreferReturn, rotationInterval int) (*Policy, error) {

	policy := Policy{
		Type: policyType,
		Rotation: &Rotation{
			Interval: rotationInterval,
		},
	}

	policyRequest := Policies{
		Metadata: PoliciesMetadata{
			CollectionType:   policyType,
			NumberOfPolicies: 1,
		},
		Policies: []Policy{policy},
	}

	policyresponse := Policies{}

	req, err := c.newRequest("PUT", fmt.Sprintf("keys/%s/policies", idOrAlias), &policyRequest)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Prefer", preferHeaders[prefer])

	_, err = c.do(ctx, req, &policyresponse)
	if err != nil {
		return nil, err
	}

	return &policyresponse.Policies[0], nil
}

// GetPolicies retrieves all policies details associated with a Key ID or alias.
func (c *Client) GetPolicies(ctx context.Context, idOrAlias string) ([]Policy, error) {
	policyresponse := Policies{}

	req, err := c.newRequest("GET", fmt.Sprintf("keys/%s/policies", idOrAlias), nil)
	if err != nil {
		return nil, err
	}

	_, err = c.do(ctx, req, &policyresponse)
	if err != nil {
		return nil, err
	}

	return policyresponse.Policies, nil
}

func (c *Client) getPolicy(ctx context.Context, id, policyType string, policyresponse *Policies) error {
	req, err := c.newRequest("GET", fmt.Sprintf("keys/%s/policies", id), nil)
	if err != nil {
		return err
	}

	v := url.Values{}
	v.Set("policy", policyType)
	req.URL.RawQuery = v.Encode()

	_, err = c.do(ctx, req, &policyresponse)
	if err != nil {
		return err
	}
	return err
}

// GetRotationPolicy method retrieves rotation policy details of a key
// For more information can refet the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-set-rotation-policy#view-rotation-policy-api
func (c *Client) GetRotationPolicy(ctx context.Context, idOrAlias string) (*Policy, error) {
	policyresponse := Policies{}

	err := c.getPolicy(ctx, idOrAlias, RotationPolicy, &policyresponse)
	if err != nil {
		return nil, err
	}

	if len(policyresponse.Policies) == 0 {
		return nil, nil
	}

	return &policyresponse.Policies[0], nil
}

// GetDualAuthDeletePolicy method retrieves dual auth delete policy details of a key
// For more information can refer the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-set-dual-auth-key-policy#view-dual-auth-key-policy-api
func (c *Client) GetDualAuthDeletePolicy(ctx context.Context, idOrAlias string) (*Policy, error) {
	policyresponse := Policies{}

	err := c.getPolicy(ctx, idOrAlias, DualAuthDelete, &policyresponse)
	if err != nil {
		return nil, err
	}

	if len(policyresponse.Policies) == 0 {
		return nil, nil
	}

	return &policyresponse.Policies[0], nil
}

func (c *Client) setPolicy(ctx context.Context, idOrAlias, policyType string, policyRequest Policies) (*Policies, error) {
	policyresponse := Policies{}

	req, err := c.newRequest("PUT", fmt.Sprintf("keys/%s/policies", idOrAlias), &policyRequest)
	if err != nil {
		return nil, err
	}

	v := url.Values{}
	v.Set("policy", policyType)
	req.URL.RawQuery = v.Encode()

	_, err = c.do(ctx, req, &policyresponse)
	if err != nil {
		return nil, err
	}
	return &policyresponse, nil
}

func (c *Client) setKeyRotationPolicy(ctx context.Context, idOrAlias string, enable *bool, rotationInterval int) (*Policy, error) {
	policy := Policy{
		Type: policyType,
		Rotation: &Rotation{
			Enabled:  enable,
			Interval: rotationInterval,
		},
	}

	policyRequest := Policies{
		Metadata: PoliciesMetadata{
			CollectionType:   policyType,
			NumberOfPolicies: 1,
		},
		Policies: []Policy{policy},
	}

	policyresponse, err := c.setPolicy(ctx, idOrAlias, RotationPolicy, policyRequest)
	if err != nil {
		return nil, err
	}

	if len(policyresponse.Policies) == 0 {
		return nil, nil
	}

	return &policyresponse.Policies[0], nil
}

func (c *Client) EnableRotationPolicy(ctx context.Context, idOrAlias string) (*Policy, error) {
	enabled := true
	return c.setKeyRotationPolicy(ctx, idOrAlias, &enabled, 0)
}

func (c *Client) DisableRotationPolicy(ctx context.Context, idOrAlias string) (*Policy, error) {
	enabled := false
	return c.setKeyRotationPolicy(ctx, idOrAlias, &enabled, 0)
}

// SetRotationPolicy updates the rotation policy associated with a key by specifying key ID  or alias and rotation interval.
// For more information can refer the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-set-rotation-policy#update-rotation-policy-api
func (c *Client) SetRotationPolicy(ctx context.Context, idOrAlias string, rotationInterval int, enabled ...bool) (*Policy, error) {
	/*
	 Setting the value of rotationInterval to -1 in case user passes 0 value as we want to retain the param `interval_month` after marshalling
	 so that we can get correct error msg from REST API saying interval_month should be between 1 to 12
	 Otherwise the param would not be sent to REST API in case of value 0 and it would throw error saying interval_month is missing
	*/
	if rotationInterval == 0 {
		rotationInterval = -1
	}
	var enable *bool
	if enabled != nil {
		enable = &enabled[0]
	}
	return c.setKeyRotationPolicy(ctx, idOrAlias, enable, rotationInterval)
}

// SetDualAuthDeletePolicy updates the dual auth delete policy by passing the key ID  or alias and enable detail
// For more information can refer the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-set-dual-auth-key-policy#create-dual-auth-key-policy-api
func (c *Client) SetDualAuthDeletePolicy(ctx context.Context, idOrAlias string, enabled bool) (*Policy, error) {
	policy := Policy{
		Type: policyType,
		DualAuth: &DualAuth{
			Enabled: &enabled,
		},
	}

	policyRequest := Policies{
		Metadata: PoliciesMetadata{
			CollectionType:   policyType,
			NumberOfPolicies: 1,
		},
		Policies: []Policy{policy},
	}

	policyresponse, err := c.setPolicy(ctx, idOrAlias, DualAuthDelete, policyRequest)
	if err != nil {
		return nil, err
	}

	if len(policyresponse.Policies) == 0 {
		return nil, nil
	}

	return &policyresponse.Policies[0], nil
}

// SetPolicies updates all policies of the key or a single policy by passing key ID.
// To set rotation policy for the key pass the setRotationPolicy parameter as true and set the rotationInterval detail.
// To set dual auth delete policy for the key pass the setDualAuthDeletePolicy parameter as true and set the dualAuthEnable detail.
// Both the policies can be set or either of the policies can be set.
func (c *Client) SetPolicies(ctx context.Context, idOrAlias string, setRotationPolicy bool, rotationInterval int, setDualAuthDeletePolicy, dualAuthEnable bool, rotationEnable ...bool) ([]Policy, error) {
	/*
	 Setting the value of rotationInterval to -1 in case user passes 0 value as we want to retain the param `interval_month` after marshalling
	 so that we can get correct error msg from REST API saying interval_month should be between 1 to 12
	 Otherwise the param would not be sent to REST API in case of value 0 and it would throw error saying interval_month is missing
	*/
	if rotationInterval == 0 {
		rotationInterval = -1
	}
	var enable *bool
	if rotationEnable != nil {
		enable = &rotationEnable[0]
	}
	policies := []Policy{}
	if setRotationPolicy {
		rotationPolicy := Policy{
			Type: policyType,
			Rotation: &Rotation{
				Enabled:  enable,
				Interval: rotationInterval,
			},
		}
		policies = append(policies, rotationPolicy)
	}
	if setDualAuthDeletePolicy {
		dulaAuthPolicy := Policy{
			Type: policyType,
			DualAuth: &DualAuth{
				Enabled: &dualAuthEnable,
			},
		}
		policies = append(policies, dulaAuthPolicy)
	}

	policyRequest := Policies{
		Metadata: PoliciesMetadata{
			CollectionType:   policyType,
			NumberOfPolicies: len(policies),
		},
		Policies: policies,
	}

	policyresponse := Policies{}

	req, err := c.newRequest("PUT", fmt.Sprintf("keys/%s/policies", idOrAlias), &policyRequest)
	if err != nil {
		return nil, err
	}
	_, err = c.do(ctx, req, &policyresponse)
	if err != nil {
		return nil, err
	}

	return policyresponse.Policies, nil
}
