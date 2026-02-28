package kp

import (
	"context"
	"fmt"
	"time"
)

const (
	kmipAdapterPath = "kmip_adapters"
	kmipAdapterType = "application/vnd.ibm.kms.kmip_adapter+json"
)

type KMIPAdapter struct {
	ID          string            `json:"id,omitempty"`
	Profile     string            `json:"profile,omitempty"`
	ProfileData map[string]string `json:"profile_data,omitempty"`
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description"`
	CreatedBy   string            `json:"created_by,omitempty"`
	CreatedAt   *time.Time        `json:"created_at,omitempty"`
	UpdatedBy   string            `json:"updated_by,omitempty"`
	UpdatedAt   *time.Time        `json:"updated_at,omitempty"`
}

type KMIPAdapters struct {
	Metadata CollectionMetadata `json:"metadata"`
	Adapters []KMIPAdapter      `json:"resources"`
}

const (
	KMIP_Profile_Native = "native_1.0"
)

// CreateKMIPAdapter method creates a KMIP Adapter with the specified profile.
func (c *Client) CreateKMIPAdapter(ctx context.Context, profileOpt CreateKMIPAdapterProfile, options ...CreateKMIPAdapterOption) (*KMIPAdapter, error) {
	newAdapter := &KMIPAdapter{}
	profileOpt(newAdapter)
	for _, opt := range options {
		opt(newAdapter)
	}
	req, err := c.newRequest("POST", kmipAdapterPath, wrapKMIPAdapter(*newAdapter))
	if err != nil {
		return nil, err
	}

	create_resp := &KMIPAdapters{}
	_, err = c.do(ctx, req, create_resp)
	if err != nil {
		return nil, err
	}
	return unwrapKMIPAdapterResp(create_resp), nil
}

// Functions to be passed into the CreateKMIPAdapter() method to specify specific fields.
type CreateKMIPAdapterOption func(*KMIPAdapter)
type CreateKMIPAdapterProfile func(*KMIPAdapter)

func WithKMIPAdapterName(name string) CreateKMIPAdapterOption {
	return func(adapter *KMIPAdapter) {
		adapter.Name = name
	}
}

func WithKMIPAdapterDescription(description string) CreateKMIPAdapterOption {
	return func(adapter *KMIPAdapter) {
		adapter.Description = description
	}
}

func WithNativeProfile(crkID string) CreateKMIPAdapterProfile {
	return func(adapter *KMIPAdapter) {
		adapter.Profile = KMIP_Profile_Native

		adapter.ProfileData = map[string]string{
			"crk_id": crkID,
		}
	}
}

type ListKmipAdaptersOptions struct {
	Limit      *uint32
	Offset     *uint32
	TotalCount *bool
	CrkID      *string
}

// GetKMIPAdapters method lists KMIP Adapters associated with a specific KP instance.
func (c *Client) GetKMIPAdapters(ctx context.Context, listOpts *ListKmipAdaptersOptions) (*KMIPAdapters, error) {
	adapters := KMIPAdapters{}
	req, err := c.newRequest("GET", kmipAdapterPath, nil)
	if err != nil {
		return nil, err
	}

	if listOpts != nil {
		values := req.URL.Query()
		if listOpts.Limit != nil {
			values.Set("limit", fmt.Sprint(*listOpts.Limit))
		}
		if listOpts.Offset != nil {
			values.Set("offset", fmt.Sprint(*listOpts.Offset))
		}
		if listOpts.TotalCount != nil {
			values.Set("totalCount", fmt.Sprint(*listOpts.TotalCount))
		}
		if listOpts.CrkID != nil {
			values.Set("crk_id", *listOpts.CrkID)
		}
		req.URL.RawQuery = values.Encode()
	}

	_, err = c.do(ctx, req, &adapters)
	if err != nil {
		return nil, err
	}

	return &adapters, nil
}

// GetKMIPAdapter method retrieves a single KMIP Adapter by name or ID.
func (c *Client) GetKMIPAdapter(ctx context.Context, nameOrID string) (*KMIPAdapter, error) {
	adapters := KMIPAdapters{}
	req, err := c.newRequest("GET", fmt.Sprintf("%s/%s", kmipAdapterPath, nameOrID), nil)
	if err != nil {
		return nil, err
	}

	_, err = c.do(ctx, req, &adapters)
	if err != nil {
		return nil, err
	}

	return unwrapKMIPAdapterResp(&adapters), nil
}

// DeletesKMIPAdapter method deletes a single KMIP Adapter by name or ID.
func (c *Client) DeleteKMIPAdapter(ctx context.Context, nameOrID string) error {
	req, err := c.newRequest("DELETE", fmt.Sprintf("%s/%s", kmipAdapterPath, nameOrID), nil)
	if err != nil {
		return err
	}

	_, err = c.do(ctx, req, nil)
	if err != nil {
		return err
	}

	return nil
}

func wrapKMIPAdapter(adapter KMIPAdapter) KMIPAdapters {
	return KMIPAdapters{
		Metadata: CollectionMetadata{
			CollectionType:  kmipAdapterType,
			CollectionTotal: 1,
		},
		Adapters: []KMIPAdapter{adapter},
	}
}

func unwrapKMIPAdapterResp(resp *KMIPAdapters) *KMIPAdapter {
	return &resp.Adapters[0]
}
