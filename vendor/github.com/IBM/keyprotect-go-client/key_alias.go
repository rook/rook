package kp

import (
	"context"
	"fmt"
	"time"
)

var (
	requestPath = "keys/%s/aliases/%s"
)

// KeyAlias represents an Alias details of a key as returned by KP API
type KeyAlias struct {
	KeyID        string     `json:"keyId,omitempty"`
	Alias        string     `json:"alias,omitempty"`
	CreatedBy    string     `json:"createdBy,omitempty"`
	CreationDate *time.Time `json:"creationDate,omitempty"`
}

// AliasesMetadata represents the metadata of a collection of aliases
type AliasesMetadata struct {
	CollectionType  string `json:"collectionType"`
	NumberOfAliases int    `json:"collectionTotal"`
}

type KeyAliases struct {
	Metadata   AliasesMetadata `json:"metadata"`
	KeyAliases []KeyAlias      `json:"resources"`
}

// CreateKeyAlias creates an alias name for a key.
// An alias name acts as an identifier just like key ID
// For more information please refer to the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-create-key-alias#create-key-alias-api
func (c *Client) CreateKeyAlias(ctx context.Context, aliasName, idOrAlias string) (*KeyAlias, error) {

	req, err := c.newRequest("POST", fmt.Sprintf(requestPath, idOrAlias, aliasName), nil)
	if err != nil {
		return nil, err
	}

	aliasesResponse := KeyAliases{}
	_, err = c.do(ctx, req, &aliasesResponse)
	if err != nil {
		return nil, err
	}

	if len(aliasesResponse.KeyAliases) == 0 {
		return nil, nil
	}

	return &aliasesResponse.KeyAliases[0], nil
}

// DeleteKeyAlias deletes an alias name associated with a key
// For more information please refer to the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-create-key-alias#delete-key-alias
func (c *Client) DeleteKeyAlias(ctx context.Context, aliasName, idOrAlias string) error {

	req, err := c.newRequest("DELETE", fmt.Sprintf(requestPath, idOrAlias, aliasName), nil)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, req, nil)

	return err
}
