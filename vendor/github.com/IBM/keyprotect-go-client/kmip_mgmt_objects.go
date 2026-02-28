package kp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	kmipObjectSubPath = "kmip_objects"
	kmipObjectType    = "application/vnd.ibm.kms.kmip_object+json"
)

type KMIPObject struct {
	ID                string     `json:"id,omitempty"`
	KMIPObjectType    int        `json:"kmip_object_type,omitempty"`
	ObjectState       int        `json:"state,omitempty"`
	CreatedByCertID   string     `json:"created_by_kmip_client_cert_id,omitempty"`
	CreatedBy         string     `json:"created_by,omitempty"`
	CreatedAt         *time.Time `json:"created_at,omitempty"`
	UpdatedByCertID   string     `json:"updated_by_kmip_client_cert_id,omitempty"`
	UpdatedBy         string     `json:"updated_by,omitempty"`
	UpdatedAt         *time.Time `json:"updated_at,omitempty"`
	DestroyedByCertID string     `json:"destroyed_by_kmip_client_cert_id,omitempty"`
	DestroyedBy       string     `json:"destroyed_by,omitempty"`
	DestroyedAt       *time.Time `json:"destroyed_at,omitempty"`
	Recoverable       *bool      `json:"recoverable,omitempty"`
}

type KMIPObjects struct {
	Metadata CollectionMetadata `json:"metadata"`
	Objects  []KMIPObject       `json:"resources"`
}

type ListKmipObjectsOptions struct {
	Limit             *uint32
	Offset            *uint32
	TotalCount        *bool
	ObjectStateFilter *[]int32
}

func (c *Client) GetKMIPObjects(ctx context.Context, adapter_id string, listOpts *ListKmipObjectsOptions) (*KMIPObjects, error) {
	objects := KMIPObjects{}
	req, err := c.newRequest("GET", fmt.Sprintf("%s/%s/%s", kmipAdapterPath, adapter_id, kmipObjectSubPath), nil)
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
		if listOpts.ObjectStateFilter != nil {
			var stateStrs []string
			for _, i := range *listOpts.ObjectStateFilter {
				stateStrs = append(stateStrs, strconv.FormatInt(int64(i), 10))
			}
			values.Set("state", strings.Join(stateStrs, ","))
		}
		req.URL.RawQuery = values.Encode()
	}

	_, err = c.do(ctx, req, &objects)
	if err != nil {
		return nil, err
	}

	return &objects, nil
}

func (c *Client) GetKMIPObject(ctx context.Context, adapter_id, object_id string) (*KMIPObject, error) {
	objects := &KMIPObjects{}
	req, err := c.newRequest("GET", fmt.Sprintf("%s/%s/%s/%s",
		kmipAdapterPath, adapter_id, kmipObjectSubPath, object_id), nil)
	if err != nil {
		return nil, err
	}

	_, err = c.do(ctx, req, objects)
	if err != nil {
		return nil, err
	}

	return unwrapKMIPObject(objects), nil
}

func (c *Client) DeleteKMIPObject(ctx context.Context, adapter_id, object_id string, opts ...RequestOpt) error {
	req, err := c.newRequest("DELETE", fmt.Sprintf("%s/%s/%s/%s",
		kmipAdapterPath, adapter_id, kmipObjectSubPath, object_id), nil)
	if err != nil {
		return err
	}

	for _, opt := range opts {
		opt(req)
	}

	_, err = c.do(ctx, req, nil)
	if err != nil {
		return err
	}

	return nil
}

func wrapKMIPObject(object KMIPObject) KMIPObjects {
	return KMIPObjects{
		Metadata: CollectionMetadata{
			CollectionType:  kmipObjectType,
			CollectionTotal: 1,
		},
		Objects: []KMIPObject{object},
	}
}

func unwrapKMIPObject(objects *KMIPObjects) *KMIPObject {
	return &objects.Objects[0]
}
