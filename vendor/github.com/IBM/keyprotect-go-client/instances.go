// Copyright 2019 IBM Corp.
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
	"net/http"
	"net/url"
	"time"
)

const (
	// AllowedNetwork defines the policy type as allowed network
	AllowedNetwork = "allowedNetwork"

	// AllowedIP defines the policy type as allowed ip that are whitelisted
	AllowedIP = "allowedIP"

	// Metrics defines the policy type as metrics
	Metrics = "metrics"
	// KeyAccess defines the policy type as key create import access
	KeyCreateImportAccess = "keyCreateImportAccess"

	// KeyAccess policy attributes
	CreateRootKey     = "CreateRootKey"
	CreateStandardKey = "CreateStandardKey"
	ImportRootKey     = "ImportRootKey"
	ImportStandardKey = "ImportStandardKey"
	EnforceToken      = "EnforceToken"
)

// InstancePolicy represents a instance-level policy of a key as returned by the KP API.
// this policy enables dual authorization for deleting a key
type InstancePolicy struct {
	CreatedBy  string     `json:"createdBy,omitempty"`
	CreatedAt  *time.Time `json:"creationDate,omitempty"`
	UpdatedAt  *time.Time `json:"lastUpdated,omitempty"`
	UpdatedBy  string     `json:"updatedBy,omitempty"`
	PolicyType string     `json:"policy_type,omitempty"`
	PolicyData PolicyData `json:"policy_data,omitempty" mapstructure:"policyData"`
}

// PolicyData contains the details of the policy type
type PolicyData struct {
	Enabled    *bool       `json:"enabled,omitempty"`
	Attributes *Attributes `json:"attributes,omitempty"`
}

// Attributes contains the details of an instance policy
type Attributes struct {
	AllowedNetwork    *string      `json:"allowed_network,omitempty"`
	AllowedIP         *IPAddresses `json:"allowed_ip,omitempty"`
	CreateRootKey     *bool        `json:"create_root_key,omitempty"`
	CreateStandardKey *bool        `json:"create_standard_key,omitempty"`
	ImportRootKey     *bool        `json:"import_root_key,omitempty"`
	ImportStandardKey *bool        `json:"import_standard_key,omitempty"`
	EnforceToken      *bool        `json:"enforce_token,omitempty"`
	IntervalMonth     *int         `json:"interval_month,omitempty"`
}

// IPAddresses ...
type IPAddresses []string

// InstancePolicies represents a collection of Policies associated with Key Protect instances.
type InstancePolicies struct {
	Metadata PoliciesMetadata `json:"metadata"`
	Policies []InstancePolicy `json:"resources"`
}

// GetDualAuthInstancePolicy retrieves the dual auth delete policy details associated with the instance
// For more information can refer the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-manage-dual-auth
func (c *Client) GetDualAuthInstancePolicy(ctx context.Context) (*InstancePolicy, error) {
	policyResponse := InstancePolicies{}

	err := c.getInstancePolicy(ctx, DualAuthDelete, &policyResponse)
	if err != nil {
		return nil, err
	}

	if len(policyResponse.Policies) == 0 {
		return nil, nil
	}
	return &policyResponse.Policies[0], nil
}

// GetAllowedNetworkInstancePolicy retrieves the allowed network policy details associated with the instance.
// For more information can refer the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-managing-network-access-policies
func (c *Client) GetAllowedNetworkInstancePolicy(ctx context.Context) (*InstancePolicy, error) {
	policyResponse := InstancePolicies{}

	err := c.getInstancePolicy(ctx, AllowedNetwork, &policyResponse)
	if err != nil {
		return nil, err
	}

	if len(policyResponse.Policies) == 0 {
		return nil, nil
	}

	return &policyResponse.Policies[0], nil
}

// GetAllowedIPInstancePolicy retrieves the allowed IP instance policy details associated with the instance.
// For more information can refer the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-manage-allowed-ip
func (c *Client) GetAllowedIPInstancePolicy(ctx context.Context) (*InstancePolicy, error) {
	policyResponse := InstancePolicies{}

	err := c.getInstancePolicy(ctx, AllowedIP, &policyResponse)
	if err != nil {
		return nil, err
	}

	if len(policyResponse.Policies) == 0 {
		return nil, nil
	}

	return &policyResponse.Policies[0], nil
}

// GetKeyCreateImportAccessInstancePolicy retrieves the key create import access policy details associated with the instance.
// For more information can refer the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-manage-keyCreateImportAccess
func (c *Client) GetKeyCreateImportAccessInstancePolicy(ctx context.Context) (*InstancePolicy, error) {
	policyResponse := InstancePolicies{}

	err := c.getInstancePolicy(ctx, KeyCreateImportAccess, &policyResponse)
	if err != nil {
		return nil, err
	}

	if len(policyResponse.Policies) == 0 {
		return nil, nil
	}

	return &policyResponse.Policies[0], nil
}

func (c *Client) getInstancePolicy(ctx context.Context, policyType string, policyResponse *InstancePolicies) error {
	req, err := c.newRequest(http.MethodGet, "instance/policies", nil)
	if err != nil {
		return err
	}

	v := url.Values{}
	v.Set("policy", policyType)
	req.URL.RawQuery = v.Encode()

	_, err = c.do(ctx, req, &policyResponse)

	return err
}

// GetMetricsInstancePolicy retrieves the metrics policy details associated with the instance
// For more information can refer the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-manage-sysdig-metrics
func (c *Client) GetMetricsInstancePolicy(ctx context.Context) (*InstancePolicy, error) {
	policyResponse := InstancePolicies{}

	err := c.getInstancePolicy(ctx, Metrics, &policyResponse)
	if err != nil {
		return nil, err
	}

	if len(policyResponse.Policies) == 0 {
		return nil, nil
	}
	return &policyResponse.Policies[0], nil
}

// GetRotationInstancePolicy retrieves the rotation policy details associated with the instance
func (c *Client) GetRotationInstancePolicy(ctx context.Context) (*InstancePolicy, error) {
	policyResponse := InstancePolicies{}

	err := c.getInstancePolicy(ctx, RotationPolicy, &policyResponse)
	if err != nil {
		return nil, err
	}

	if len(policyResponse.Policies) == 0 {
		return nil, nil
	}
	return &policyResponse.Policies[0], nil
}

// GetInstancePolicies retrieves all policies of an Instance.
func (c *Client) GetInstancePolicies(ctx context.Context) ([]InstancePolicy, error) {
	policyresponse := InstancePolicies{}

	req, err := c.newRequest(http.MethodGet, "instance/policies", nil)
	if err != nil {
		return nil, err
	}

	_, err = c.do(ctx, req, &policyresponse)
	if err != nil {
		return nil, err
	}

	return policyresponse.Policies, nil
}

func (c *Client) setInstancePolicy(ctx context.Context, policyType string, policyRequest InstancePolicies) error {
	req, err := c.newRequest(http.MethodPut, "instance/policies", &policyRequest)
	if err != nil {
		return err
	}

	v := url.Values{}
	v.Set("policy", policyType)
	req.URL.RawQuery = v.Encode()

	policiesResponse := InstancePolicies{}
	_, err = c.do(ctx, req, &policiesResponse)

	return err
}

// SetDualAuthInstancePolicy updates the dual auth delete policy details associated with an instance
// For more information can refer the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-manage-dual-auth
func (c *Client) SetDualAuthInstancePolicy(ctx context.Context, enable bool) error {
	policy := InstancePolicy{
		PolicyType: DualAuthDelete,
		PolicyData: PolicyData{
			Enabled: &enable,
		},
	}

	policyRequest := InstancePolicies{
		Metadata: PoliciesMetadata{
			CollectionType:   policyType,
			NumberOfPolicies: 1,
		},
		Policies: []InstancePolicy{policy},
	}

	err := c.setInstancePolicy(ctx, DualAuthDelete, policyRequest)

	return err
}

func addRotationInstancePolicyData(enable bool, intervalMonth *int) (InstancePolicy, error) {

	rotationPolicyData := InstancePolicy{
		PolicyType: RotationPolicy,
		PolicyData: PolicyData{
			Enabled: &enable,
		},
	}

	if enable && intervalMonth == nil {
		return InstancePolicy{}, fmt.Errorf("Interval Month is required to enable rotation instance policy")
	} else if !enable && intervalMonth != nil {
		return InstancePolicy{}, fmt.Errorf("Interval Month should only be provided if the policy is being enabled")
	} else if intervalMonth != nil {
		rotationPolicyData.PolicyData.Attributes = &Attributes{
			IntervalMonth: intervalMonth,
		}
	}

	return rotationPolicyData, nil
}

// SetRotationInstancePolicy updates the rotation instance policy details associated with an instance.
func (c *Client) SetRotationInstancePolicy(ctx context.Context, enable bool, intervalMonth *int) error {

	rotationPolicyData, err := addRotationInstancePolicyData(enable, intervalMonth)
	if err != nil {
		return err
	}

	policyRequest := InstancePolicies{
		Metadata: PoliciesMetadata{
			CollectionType:   policyType,
			NumberOfPolicies: 1,
		},
		Policies: []InstancePolicy{rotationPolicyData},
	}

	err = c.setInstancePolicy(ctx, RotationPolicy, policyRequest)

	return err
}

// SetAllowedIPInstancePolices updates the allowed IP instance policy details associated with an instance.
// For more information can refet to the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-manage-allowed-ip
func (c *Client) SetAllowedIPInstancePolicy(ctx context.Context, enable bool, allowedIPs []string) error {

	policy := InstancePolicy{
		PolicyType: AllowedIP,
		PolicyData: PolicyData{
			Enabled: &enable,
		},
	}

	// The IP address validation is performed by the key protect service.
	if enable && len(allowedIPs) != 0 {
		policy.PolicyData.Attributes = &Attributes{}
		ips := IPAddresses(allowedIPs)
		policy.PolicyData.Attributes.AllowedIP = &ips
	} else if enable && len(allowedIPs) == 0 {
		return fmt.Errorf("Please provide at least 1 IP subnet specified with CIDR notation")
	} else if !enable && len(allowedIPs) != 0 {
		return fmt.Errorf("IP address list should only be provided if the policy is being enabled")
	}

	policyRequest := InstancePolicies{
		Metadata: PoliciesMetadata{
			CollectionType:   policyType,
			NumberOfPolicies: 1,
		},
		Policies: []InstancePolicy{policy},
	}
	err := c.setInstancePolicy(ctx, AllowedIP, policyRequest)

	return err
}

// SetAllowedNetWorkInstancePolicy updates the allowed network policy details associated with an instance
// For more information can refer to the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-managing-network-access-policies
func (c *Client) SetAllowedNetworkInstancePolicy(ctx context.Context, enable bool, networkType string) error {
	policy := InstancePolicy{
		PolicyType: AllowedNetwork,
		PolicyData: PolicyData{
			Enabled:    &enable,
			Attributes: &Attributes{},
		},
	}
	if networkType != "" {
		policy.PolicyData.Attributes.AllowedNetwork = &networkType
	}

	policyRequest := InstancePolicies{
		Metadata: PoliciesMetadata{
			CollectionType:   policyType,
			NumberOfPolicies: 1,
		},
		Policies: []InstancePolicy{policy},
	}

	err := c.setInstancePolicy(ctx, AllowedNetwork, policyRequest)

	return err
}

// SetMetricsInstancePolicy updates the metrics policy details associated with an instance
// For more information can refer the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-manage-sysdig-metrics
func (c *Client) SetMetricsInstancePolicy(ctx context.Context, enable bool) error {
	policy := InstancePolicy{
		PolicyType: Metrics,
		PolicyData: PolicyData{
			Enabled: &enable,
		},
	}

	policyRequest := InstancePolicies{
		Metadata: PoliciesMetadata{
			CollectionType:   policyType,
			NumberOfPolicies: 1,
		},
		Policies: []InstancePolicy{policy},
	}

	err := c.setInstancePolicy(ctx, Metrics, policyRequest)
	if err != nil {
		return err
	}

	return err
}

// SetKeyCreateImportAccessInstancePolicy updates the key create import access policy details associated with an instance.
// For more information, please refer to the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-manage-keyCreateImportAccess
func (c *Client) SetKeyCreateImportAccessInstancePolicy(ctx context.Context, enable bool, attributes map[string]bool) error {
	policy := InstancePolicy{
		PolicyType: KeyCreateImportAccess,
		PolicyData: PolicyData{
			Enabled: &enable,
		},
	}

	if enable {
		policy.PolicyData.Attributes = &Attributes{}
		a := policy.PolicyData.Attributes
		if val, ok := attributes[CreateRootKey]; ok {
			a.CreateRootKey = &val
		}
		if val, ok := attributes[CreateStandardKey]; ok {
			a.CreateStandardKey = &val
		}
		if val, ok := attributes[ImportRootKey]; ok {
			a.ImportRootKey = &val
		}
		if val, ok := attributes[ImportStandardKey]; ok {
			a.ImportStandardKey = &val
		}
		if val, ok := attributes[EnforceToken]; ok {
			a.EnforceToken = &val
		}
	}

	policyRequest := InstancePolicies{
		Metadata: PoliciesMetadata{
			CollectionType:   policyType,
			NumberOfPolicies: 1,
		},
		Policies: []InstancePolicy{policy},
	}

	err := c.setInstancePolicy(ctx, KeyCreateImportAccess, policyRequest)

	return err
}

// BasicPolicyData defines the attribute input for the policy that supports only enabled parameter
type BasicPolicyData struct {
	Enabled bool
}

// AllowedNetworkPolicyData defines the attribute input for the Allowed Network instance policy
type AllowedNetworkPolicyData struct {
	Enabled bool
	Network string
}

// AllowedIPPolicyData defines the attribute input for the Allowed IP instance policy
type AllowedIPPolicyData struct {
	Enabled     bool
	IPAddresses *IPAddresses
}

// KeyAccessInstancePolicyData defines the attribute input for the Key Create Import Access instance policy
type KeyCreateImportAccessInstancePolicy struct {
	Enabled    bool
	Attributes *KeyCreateImportAccessInstancePolicyAttributes
}

type KeyCreateImportAccessInstancePolicyAttributes struct {
	CreateRootKey     *bool
	CreateStandardKey *bool
	ImportRootKey     *bool
	ImportStandardKey *bool
	EnforceToken      *bool
}

type RotationPolicyData struct {
	Enabled       bool
	IntervalMonth *int
}

// MultiplePolicies defines the input for the SetInstancPolicies method that can hold multiple policy details
type MultiplePolicies struct {
	DualAuthDelete        *BasicPolicyData
	AllowedNetwork        *AllowedNetworkPolicyData
	AllowedIP             *AllowedIPPolicyData
	Metrics               *BasicPolicyData
	KeyCreateImportAccess *KeyCreateImportAccessInstancePolicy
	Rotation              *RotationPolicyData
}

// SetInstancePolicies updates single or multiple policy details of an instance.
func (c *Client) SetInstancePolicies(ctx context.Context, policies MultiplePolicies) error {
	var resPolicies []InstancePolicy

	if policies.DualAuthDelete != nil {
		policy := InstancePolicy{
			PolicyType: DualAuthDelete,
			PolicyData: PolicyData{
				Enabled: &(policies.DualAuthDelete.Enabled),
			},
		}
		resPolicies = append(resPolicies, policy)
	}

	if policies.AllowedNetwork != nil {
		policy := InstancePolicy{
			PolicyType: AllowedNetwork,
			PolicyData: PolicyData{
				Enabled: &(policies.AllowedNetwork.Enabled),
				// due to legacy reasons, the allowed_network policy requires attribute to always be specified
				Attributes: &Attributes{
					AllowedNetwork: &(policies.AllowedNetwork.Network),
				},
			},
		}
		resPolicies = append(resPolicies, policy)
	}

	if policies.AllowedIP != nil {
		policy := InstancePolicy{
			PolicyType: AllowedIP,
			PolicyData: PolicyData{
				Enabled: &(policies.AllowedIP.Enabled),
				Attributes: &Attributes{
					AllowedIP: policies.AllowedIP.IPAddresses,
				},
			},
		}
		resPolicies = append(resPolicies, policy)
	}

	if policies.Metrics != nil {
		policy := InstancePolicy{
			PolicyType: Metrics,
			PolicyData: PolicyData{
				Enabled: &(policies.Metrics.Enabled),
			},
		}
		resPolicies = append(resPolicies, policy)
	}

	if policies.KeyCreateImportAccess != nil {
		policy := InstancePolicy{
			PolicyType: KeyCreateImportAccess,
			PolicyData: PolicyData{
				Enabled: &(policies.KeyCreateImportAccess.Enabled),
			},
		}

		if attr := policies.KeyCreateImportAccess.Attributes; attr != nil {
			policy.PolicyData.Attributes = &Attributes{
				CreateRootKey:     attr.CreateRootKey,
				CreateStandardKey: attr.CreateStandardKey,
				ImportRootKey:     attr.ImportRootKey,
				ImportStandardKey: attr.ImportStandardKey,
				EnforceToken:      attr.EnforceToken,
			}
		}

		resPolicies = append(resPolicies, policy)
	}

	if policies.Rotation != nil {
		policy, err := addRotationInstancePolicyData(policies.Rotation.Enabled, policies.Rotation.IntervalMonth)
		if err != nil {
			return err
		}

		resPolicies = append(resPolicies, policy)
	}

	policyRequest := InstancePolicies{
		Metadata: PoliciesMetadata{
			CollectionType:   policyType,
			NumberOfPolicies: len(resPolicies),
		},
		Policies: resPolicies,
	}

	policyresponse := Policies{}

	req, err := c.newRequest(http.MethodPut, "instance/policies", &policyRequest)
	if err != nil {
		return err
	}

	_, err = c.do(ctx, req, &policyresponse)

	return err
}

type portsMetadata struct {
	CollectionType string `json:"collectionType"`
	NumberOfPorts  int    `json:"collectionTotal"`
}

type portResponse struct {
	Metadata portsMetadata `json:"metadata"`
	Ports    []privatePort `json:"resources"`
}
type privatePort struct {
	PrivatePort int `json:"private_endpoint_port,omitempty"`
}

// GetAllowedIPPrivateNetworkPort retrieves the private endpoint port assigned to allowed ip policy.
func (c *Client) GetAllowedIPPrivateNetworkPort(ctx context.Context) (int, error) {
	var portResponse portResponse

	req, err := c.newRequest(http.MethodGet, "instance/allowed_ip_port", nil)
	if err != nil {
		return 0, err
	}

	_, err = c.do(ctx, req, &portResponse)
	if err != nil {
		return 0, err
	}

	if len(portResponse.Ports) == 0 {
		return 0, fmt.Errorf("No port number available. Please check the instance has an enabled allowedIP policy")
	}
	return portResponse.Ports[0].PrivatePort, nil
}
