package kp

import (
	"context"
	"fmt"
	"time"
)

const (
	kmipClientCertSubPath = "certificates"
	kmipClientCertType    = "application/vnd.ibm.kms.kmip_client_certificate+json"
)

type KMIPClientCertificate struct {
	ID          string     `json:"id,omitempty"`
	Name        string     `json:"name,omitempty"`
	Certificate string     `json:"certificate,omitempty"`
	CreatedBy   string     `json:"created_by,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
}

type KMIPClientCertificates struct {
	Metadata     CollectionMetadata      `json:"metadata"`
	Certificates []KMIPClientCertificate `json:"resources"`
}

// CreateKMIPClientCertificate registers/creates a KMIP PEM format certificate
// for use with a specific KMIP adapter.
// cert_payload is the string representation of
// the certificate to be associated with the KMIP Adapter in PEM format.
// It should explicitly have the BEGIN CERTIFICATE and END CERTIFICATE tags.
// Regex: ^\s*-----BEGIN CERTIFICATE-----[A-Za-z0-9+\/\=\r\n]+-----END CERTIFICATE-----\s*$
func (c *Client) CreateKMIPClientCertificate(ctx context.Context, adapter_nameOrID, cert_payload string, opts ...CreateKMIPClientCertOption) (*KMIPClientCertificate, error) {
	newCert := &KMIPClientCertificate{
		Certificate: cert_payload,
	}
	for _, opt := range opts {
		opt(newCert)
	}
	req, err := c.newRequest("POST", fmt.Sprintf("%s/%s/%s", kmipAdapterPath, adapter_nameOrID, kmipClientCertSubPath), wrapKMIPClientCert(*newCert))
	if err != nil {
		return nil, err
	}
	certResp := &KMIPClientCertificates{}
	_, err = c.do(ctx, req, certResp)
	if err != nil {
		return nil, err
	}

	return unwrapKMIPClientCert(certResp), nil
}

type CreateKMIPClientCertOption func(*KMIPClientCertificate)

func WithKMIPClientCertName(name string) CreateKMIPClientCertOption {
	return func(cert *KMIPClientCertificate) {
		cert.Name = name
	}
}

// GetKMIPClientCertificates lists all certificates associated with a KMIP adapter
func (c *Client) GetKMIPClientCertificates(ctx context.Context, adapter_nameOrID string, listOpts *ListOptions) (*KMIPClientCertificates, error) {
	certs := KMIPClientCertificates{}
	req, err := c.newRequest("GET", fmt.Sprintf("%s/%s/%s", kmipAdapterPath, adapter_nameOrID, kmipClientCertSubPath), nil)
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
		req.URL.RawQuery = values.Encode()
	}

	_, err = c.do(ctx, req, &certs)
	if err != nil {
		return nil, err
	}

	return &certs, nil
}

// GetKMIPClientCertificate gets a single certificate associated with a KMIP adapter
func (c *Client) GetKMIPClientCertificate(ctx context.Context, adapter_nameOrID, cert_nameOrID string) (*KMIPClientCertificate, error) {
	certs := &KMIPClientCertificates{}
	req, err := c.newRequest("GET", fmt.Sprintf("%s/%s/%s/%s",
		kmipAdapterPath, adapter_nameOrID, kmipClientCertSubPath, cert_nameOrID), nil)
	if err != nil {
		return nil, err
	}

	_, err = c.do(ctx, req, certs)
	if err != nil {
		return nil, err
	}

	return unwrapKMIPClientCert(certs), nil
}

// DeleteKMIPClientCertificate deletes a single certificate
func (c *Client) DeleteKMIPClientCertificate(ctx context.Context, adapter_nameOrID, cert_nameOrID string) error {
	req, err := c.newRequest("DELETE", fmt.Sprintf("%s/%s/%s/%s",
		kmipAdapterPath, adapter_nameOrID, kmipClientCertSubPath, cert_nameOrID), nil)
	if err != nil {
		return err
	}

	_, err = c.do(ctx, req, nil)
	if err != nil {
		return err
	}

	return nil
}

func wrapKMIPClientCert(cert KMIPClientCertificate) KMIPClientCertificates {
	return KMIPClientCertificates{
		Metadata: CollectionMetadata{
			CollectionType:  kmipClientCertType,
			CollectionTotal: 1,
		},
		Certificates: []KMIPClientCertificate{cert},
	}
}

func unwrapKMIPClientCert(certs *KMIPClientCertificates) *KMIPClientCertificate {
	return &certs.Certificates[0]
}
