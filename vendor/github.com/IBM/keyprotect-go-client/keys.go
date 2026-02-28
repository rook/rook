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
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	ReturnMinimal        PreferReturn = 0
	ReturnRepresentation PreferReturn = 1

	keyType = "application/vnd.ibm.kms.key+json"
)

var (
	preferHeaders               = []string{"return=minimal", "return=representation"}
	keysPath                    = "keys"
	keysWithPolicyOverridesPath = "keys_with_policy_overrides"
)

// PreferReturn designates the value for the "Prefer" header.
type PreferReturn int

type KeyState uint32

// https://cloud.ibm.com/docs/key-protect?topic=key-protect-key-states
const (
	Active KeyState = iota + 1
	Suspended
	Deactivated
	_
	Destroyed
)

// Key represents a key as returned by the KP API.
type Key struct {
	ID                  string      `json:"id,omitempty"`
	Name                string      `json:"name,omitempty"`
	Description         string      `json:"description,omitempty"`
	Type                string      `json:"type,omitempty"`
	Tags                []string    `json:"Tags,omitempty"`
	Aliases             []string    `json:"aliases,omitempty"`
	AlgorithmType       string      `json:"algorithmType,omitempty"`
	CreatedBy           string      `json:"createdBy,omitempty"`
	CreationDate        *time.Time  `json:"creationDate,omitempty"`
	LastUpdateDate      *time.Time  `json:"lastUpdateDate,omitempty"`
	LastRotateDate      *time.Time  `json:"lastRotateDate,omitempty"`
	KeyVersion          *KeyVersion `json:"keyVersion,omitempty" mapstructure:keyVersion`
	KeyRingID           string      `json:"keyRingID,omitempty"`
	Extractable         bool        `json:"extractable"`
	Expiration          *time.Time  `json:"expirationDate,omitempty"`
	Imported            bool        `json:"imported,omitempty"`
	Payload             string      `json:"payload,omitempty"`
	State               int         `json:"state,omitempty"`
	EncryptionAlgorithm string      `json:"encryptionAlgorithm,omitempty"`
	CRN                 string      `json:"crn,omitempty"`
	EncryptedNonce      string      `json:"encryptedNonce,omitempty"`
	IV                  string      `json:"iv,omitempty"`
	Deleted             *bool       `json:"deleted,omitempty"`
	DeletedBy           *string     `json:"deletedBy,omitempty"`
	DeletionDate        *time.Time  `json:"deletionDate,omitempty"`
	PurgeAllowed        *bool       `json:"purgeAllowed,omitempty"`
	PurgeAllowedFrom    *time.Time  `json:"purgeAllowedFrom,omitempty"`
	PurgeScheduledOn    *time.Time  `json:"purgeScheduledOn,omitempty"`
	DualAuthDelete      *DualAuth   `json:"dualAuthDelete,omitempty"`
	Rotation            *Rotation   `json:"rotation,omitempty"`
}

// KeysMetadata represents the metadata of a collection of keys.
type KeysMetadata struct {
	CollectionType string `json:"collectionType"`
	NumberOfKeys   int    `json:"collectionTotal"`
}

// Keys represents a collection of Keys.
type Keys struct {
	Metadata KeysMetadata `json:"metadata"`
	Keys     []Key        `json:"resources"`
}

type KeyVersionsMetadata struct {
	CollectionType  string  `json:"collectionType"`
	CollectionTotal *uint32 `json:"collectionTotal"`
	TotalCount      *uint32 `json:"totalCount,omitempty"`
}

type KeyVersions struct {
	Metadata   KeyVersionsMetadata `json:"metadata"`
	KeyVersion []KeyVersion        `json:"resources"`
}

// KeysActionRequest represents request parameters for a key action
// API call.
type KeysActionRequest struct {
	PlainText           string      `json:"plaintext,omitempty"`
	AAD                 []string    `json:"aad,omitempty"`
	CipherText          string      `json:"ciphertext,omitempty"`
	Payload             string      `json:"payload,omitempty"`
	EncryptedNonce      string      `json:"encryptedNonce,omitempty"`
	IV                  string      `json:"iv,omitempty"`
	EncryptionAlgorithm string      `json:"encryptionAlgorithm,omitempty"`
	KeyVersion          *KeyVersion `json:"keyVersion,,omitempty"`
}

type KeyActionResponse struct {
	PlainText  string      `json:"plaintext,omitempty"`
	CipherText string      `json:"ciphertext,omitempty"`
	KeyVersion *KeyVersion `json:"keyVersion,,omitempty"`
}

type KeyVersion struct {
	ID           string     `json:"id,omitempty"`
	CreationDate *time.Time `json:"creationDate,omitempty"`
}

// This function returns a string so we can pass extra info not in the key struct if needed
type CreateKeyOption func(k *Key)

func WithExpiration(expiration *time.Time) CreateKeyOption {
	return func(key *Key) {
		key.Expiration = expiration
	}
}

func WithDescription(description string) CreateKeyOption {
	return func(key *Key) {
		key.Description = description
	}
}

func WithPayload(payload string, encryptedNonce, iv *string, sha1 bool) CreateKeyOption {
	return func(key *Key) {
		key.Payload = payload
		if !key.Extractable {
			hasNonce := encryptedNonce != nil && *encryptedNonce != ""
			hasIV := iv != nil && *iv != ""
			if hasNonce {
				key.EncryptedNonce = *encryptedNonce
			}
			if hasIV {
				key.IV = *iv
			}
			// Encryption algo field is only for secure import.
			// Only included it if either nonce or IV are specified.
			// API will error if only one of IV or nonce are specified but the other is empty.
			if hasNonce || hasIV {
				algorithm := AlgorithmRSAOAEP256
				if sha1 {
					algorithm = AlgorithmRSAOAEP1
				}
				key.EncryptionAlgorithm = algorithm
			}
		}
	}
}

func WithAliases(aliases []string) CreateKeyOption {
	return func(key *Key) {
		key.Aliases = aliases
	}
}

func WithTags(tags []string) CreateKeyOption {
	return func(key *Key) {
		key.Tags = tags
	}
}

func (c *Client) CreateKeyWithOptions(ctx context.Context, name string, extractable bool, options ...CreateKeyOption) (*Key, error) {
	key := &Key{
		Name:        name,
		Type:        keyType,
		Extractable: extractable,
	}
	for _, opt := range options {
		opt(key)
	}
	return c.createKeyResource(ctx, *key, keysPath)
}

func (c *Client) CreateKeyWithPolicyOverridesWithOptions(ctx context.Context, name string, extractable bool, policy Policy, options ...CreateKeyOption) (*Key, error) {
	key := &Key{
		Name:        name,
		Type:        keyType,
		Extractable: extractable,
	}
	for _, opt := range options {
		opt(key)
	}
	/*
		Setting the value of rotationInterval to -1 in case user passes 0 value
		as we want to retain the param `interval_month` after marshalling
		so that we can get correct error msg from REST API saying interval_month should be between 1 to 12
		Otherwise the param would not be sent to REST API in case of value 0
		and it would throw error saying interval_month is missing
	*/
	if policy.Rotation != nil && policy.Rotation.Interval == 0 {
		policy.Rotation.Interval = -1
	}
	key.Rotation = policy.Rotation
	key.DualAuthDelete = policy.DualAuth

	return c.createKeyResource(ctx, *key, keysWithPolicyOverridesPath)
}

// CreateKey creates a new KP key.
func (c *Client) CreateKey(ctx context.Context, name string, expiration *time.Time, extractable bool) (*Key, error) {
	return c.CreateKeyWithOptions(ctx, name, extractable, WithExpiration(expiration))
}

// CreateImportedKey creates a new KP key from the given key material.
func (c *Client) CreateImportedKey(ctx context.Context, name string, expiration *time.Time, payload, encryptedNonce, iv string, extractable bool) (*Key, error) {
	return c.CreateKeyWithOptions(ctx, name, extractable,
		WithExpiration(expiration),
		WithPayload(payload, &encryptedNonce, &iv, false),
	)
}

// CreateImportedKeyWithSHA1 creates a new KP key from the given key material
// using RSAES OAEP SHA 1 as encryption algorithm.
func (c *Client) CreateImportedKeyWithSHA1(ctx context.Context, name string, expiration *time.Time,
	payload, encryptedNonce, iv string, extractable bool, aliases []string) (*Key, error) {
	return c.CreateKeyWithOptions(ctx, name, extractable,
		WithExpiration(expiration),
		WithPayload(payload, &encryptedNonce, &iv, true),
		WithAliases(aliases),
	)
}

// CreateRootKey creates a new, non-extractable key resource without
// key material.
func (c *Client) CreateRootKey(ctx context.Context, name string, expiration *time.Time) (*Key, error) {
	return c.CreateKey(ctx, name, expiration, false)
}

// CreateStandardKey creates a new, extractable key resource without
// key material.
func (c *Client) CreateStandardKey(ctx context.Context, name string, expiration *time.Time) (*Key, error) {
	return c.CreateKey(ctx, name, expiration, true)
}

// CreateImportedRootKey creates a new, non-extractable key resource
// with the given key material.
func (c *Client) CreateImportedRootKey(ctx context.Context, name string, expiration *time.Time, payload, encryptedNonce, iv string) (*Key, error) {
	return c.CreateImportedKey(ctx, name, expiration, payload, encryptedNonce, iv, false)
}

// CreateStandardKey creates a new, extractable key resource with the
// given key material.
func (c *Client) CreateImportedStandardKey(ctx context.Context, name string, expiration *time.Time, payload string) (*Key, error) {
	return c.CreateImportedKey(ctx, name, expiration, payload, "", "", true)
}

// CreateKeyWithAliaes creats a new key with alias names. A key can have a maximum of 5 alias names.
// For more information please refer to the links below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-create-root-keys#create-root-key-api
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-create-standard-keys#create-standard-key-api
func (c *Client) CreateKeyWithAliases(ctx context.Context, name string, expiration *time.Time, extractable bool, aliases []string) (*Key, error) {
	return c.CreateImportedKeyWithAliases(ctx, name, expiration, "", "", "", extractable, aliases)
}

// CreateImportedKeyWithAliases creates a new key with alias name and provided key material. A key can have a maximum of 5 alias names
// When importing root keys with import-token encryptedNonce and iv need to passed along with payload.
// Standard Keys cannot be imported with an import token hence only payload is required.
// For more information please refer to the links below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-import-root-keys#import-root-key-api
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-import-standard-keys#import-standard-key-gui
func (c *Client) CreateImportedKeyWithAliases(ctx context.Context, name string, expiration *time.Time,
	payload, encryptedNonce, iv string, extractable bool, aliases []string) (*Key, error) {
	return c.CreateKeyWithOptions(ctx, name, extractable,
		WithExpiration(expiration),
		WithPayload(payload, &encryptedNonce, &iv, false),
		WithAliases(aliases),
	)
}

// CreateImportedKeyWithPolicyOverridesWithSHA1 creates a new KP key with policy overrides from the given key material
// and key policy details using RSAES OAEP SHA 1 as encryption algorithm.
func (c *Client) CreateImportedKeyWithPolicyOverridesWithSHA1(ctx context.Context, name string, expiration *time.Time,
	payload, encryptedNonce, iv string, extractable bool, aliases []string, policy Policy) (*Key, error) {
	return c.CreateKeyWithPolicyOverridesWithOptions(ctx, name, extractable, policy,
		WithExpiration(expiration),
		WithPayload(payload, &encryptedNonce, &iv, true),
		WithAliases(aliases),
	)
}

// CreateKeyWithPolicyOverrides creates a new KP key with given key policy details
func (c *Client) CreateKeyWithPolicyOverrides(ctx context.Context, name string, expiration *time.Time, extractable bool, aliases []string, policy Policy) (*Key, error) {
	return c.CreateKeyWithPolicyOverridesWithOptions(ctx, name, extractable, policy,
		WithExpiration(expiration),
		WithAliases(aliases),
	)
}

// CreateImportedKeyWithPolicyOverrides creates a new Imported KP key from the given key material and with given key policy details
func (c *Client) CreateImportedKeyWithPolicyOverrides(ctx context.Context, name string, expiration *time.Time,
	payload, encryptedNonce, iv string, extractable bool, aliases []string, policy Policy) (*Key, error) {
	return c.CreateKeyWithPolicyOverridesWithOptions(ctx, name, extractable, policy,
		WithExpiration(expiration),
		WithPayload(payload, &encryptedNonce, &iv, false),
		WithAliases(aliases),
	)
}

// CreateRootKeyWithPolicyOverrides creates a new, non-extractable key resource without key material and with given key policy details
func (c *Client) CreateRootKeyWithPolicyOverrides(ctx context.Context, name string, expiration *time.Time, aliases []string, policy Policy) (*Key, error) {
	return c.CreateKeyWithPolicyOverrides(ctx, name, expiration, false, aliases, policy)
}

// CreateStandardKeyWithPolicyOverrides creates a new, extractable key resource without key material and with given key policy details
func (c *Client) CreateStandardKeyWithPolicyOverrides(ctx context.Context, name string, expiration *time.Time, aliases []string, policy Policy) (*Key, error) {
	return c.CreateKeyWithPolicyOverrides(ctx, name, expiration, true, aliases, policy)
}

// CreateImportedRootKeyWithPolicyOverrides creates a new, non-extractable key resource with the given key material and with given key policy details
func (c *Client) CreateImportedRootKeyWithPolicyOverrides(ctx context.Context, name string, expiration *time.Time,
	payload, encryptedNonce, iv string, aliases []string, policy Policy) (*Key, error) {
	return c.CreateImportedKeyWithPolicyOverrides(ctx, name, expiration, payload, encryptedNonce, iv, false, aliases, policy)
}

// CreateImportedStandardKeyWithPolicyOverrides creates a new, extractable key resource with the given key material and with given key policy details
func (c *Client) CreateImportedStandardKeyWithPolicyOverrides(ctx context.Context, name string, expiration *time.Time,
	payload string, aliases []string, policy Policy) (*Key, error) {
	return c.CreateImportedKeyWithPolicyOverrides(ctx, name, expiration, payload, "", "", true, aliases, policy)
}

func (c *Client) createKeyResource(ctx context.Context, key Key, path string) (*Key, error) {
	keysRequest := Keys{
		Metadata: KeysMetadata{
			CollectionType: keyType,
			NumberOfKeys:   1,
		},
		Keys: []Key{key},
	}

	req, err := c.newRequest(http.MethodPost, path, &keysRequest)
	if err != nil {
		return nil, err
	}
	keysResponse := Keys{}
	if _, err := c.do(ctx, req, &keysResponse); err != nil {
		return nil, err
	}

	return &keysResponse.Keys[0], nil
}

// SetKeyRing method transfers a key associated with one key ring to another key ring
// For more information please refer to the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-grouping-keys#transfer-key-key-ring
func (c *Client) SetKeyRing(ctx context.Context, idOrAlias, newKeyRingID string) (*Key, error) {
	if idOrAlias == "" {
		return nil, fmt.Errorf("Please provide a valid key ID or alias")
	}

	if newKeyRingID == "" {
		return nil, fmt.Errorf("Please provide a valid key ring id")
	}

	keyRingRequestBody := struct {
		KeyRingID string
	}{
		KeyRingID: newKeyRingID,
	}

	req, err := c.newRequest("PATCH", fmt.Sprintf("keys/%s", idOrAlias), keyRingRequestBody)
	if err != nil {
		return nil, err
	}

	response := Keys{}
	if _, err := c.do(ctx, req, &response); err != nil {
		return nil, err
	}
	return &response.Keys[0], nil
}

// GetKeys retrieves a collection of keys that can be paged through.
func (c *Client) GetKeys(ctx context.Context, limit int, offset int) (*Keys, error) {
	if limit == 0 {
		limit = 2000
	}

	req, err := c.newRequest("GET", "keys", nil)
	if err != nil {
		return nil, err
	}

	v := url.Values{}
	v.Set("limit", strconv.Itoa(limit))
	v.Set("offset", strconv.Itoa(offset))
	req.URL.RawQuery = v.Encode()

	keys := Keys{}
	_, err = c.do(ctx, req, &keys)
	if err != nil {
		return nil, err
	}

	return &keys, nil
}

// GetKey retrieves a key by ID or alias name.
// For more information on Key Alias please refer to the link below
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-retrieve-key
func (c *Client) GetKey(ctx context.Context, idOrAlias string) (*Key, error) {
	return c.getKey(ctx, idOrAlias, "keys/%s")
}

// GetKeyMetadata retrieves the metadata of a Key by ID or alias name.
// Note that the "/api/v2/keys/{id}/metadata" API does not return the payload,
// therefore the payload attribute in the Key pointer will always be empty.
// If you need the payload, you need to use the GetKey() function with the
// correct service access role.
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-manage-access#service-access-roles
func (c *Client) GetKeyMetadata(ctx context.Context, idOrAlias string) (*Key, error) {
	return c.getKey(ctx, idOrAlias, "keys/%s/metadata")
}

func (c *Client) getKey(ctx context.Context, idOrAlias string, path string) (*Key, error) {
	keys := Keys{}

	req, err := c.newRequest("GET", fmt.Sprintf(path, idOrAlias), nil)
	if err != nil {
		return nil, err
	}

	_, err = c.do(ctx, req, &keys)
	if err != nil {
		return nil, err
	}

	return &keys.Keys[0], nil
}

type CallOpt interface{}

type ForceOpt struct {
	Force bool
}

// ListKeyVersionsOptions struct to add the query parameters for the ListKeyVersions function
type ListKeyVersionsOptions struct {
	Limit        *uint32
	Offset       *uint32
	TotalCount   *bool
	AllKeyStates *bool
}

// ListKeyVersions gets all the versions of the key resource by specifying ID of the key and/or optional parameters
// https://cloud.ibm.com/apidocs/key-protect#getkeyversions
func (c *Client) ListKeyVersions(ctx context.Context, idOrAlias string, listKeyVersionsOptions *ListKeyVersionsOptions) (*KeyVersions, error) {
	keyVersion := KeyVersions{}
	// forming the request
	req, err := c.newRequest("GET", fmt.Sprintf("keys/%s/versions", idOrAlias), nil)
	if err != nil {
		return nil, err
	}

	// extracting the query parameters and encoding the same in the request url
	if listKeyVersionsOptions != nil {
		values := req.URL.Query()
		if listKeyVersionsOptions.Limit != nil {
			values.Set("limit", fmt.Sprint(*listKeyVersionsOptions.Limit))
		}
		if listKeyVersionsOptions.Offset != nil {
			values.Set("offset", fmt.Sprint(*listKeyVersionsOptions.Offset))
		}
		if listKeyVersionsOptions.TotalCount != nil {
			values.Set("totalCount", fmt.Sprint(*listKeyVersionsOptions.TotalCount))
		}
		if listKeyVersionsOptions.AllKeyStates != nil {
			values.Set("allKeyStates", fmt.Sprint(*listKeyVersionsOptions.AllKeyStates))
		}
		req.URL.RawQuery = values.Encode()
	}

	//making a request
	_, err = c.do(ctx, req, &keyVersion)
	if err != nil {
		return nil, err
	}

	return &keyVersion, nil
}

// DeleteKey deletes a key resource by specifying the ID of the key.
func (c *Client) DeleteKey(ctx context.Context, idOrAlias string, prefer PreferReturn, callOpts ...CallOpt) (*Key, error) {

	req, err := c.newRequest("DELETE", fmt.Sprintf("keys/%s", idOrAlias), nil)
	if err != nil {
		return nil, err
	}

	for _, opt := range callOpts {
		switch v := opt.(type) {
		case ForceOpt:
			params := url.Values{}
			params.Set("force", strconv.FormatBool(v.Force))
			req.URL.RawQuery = params.Encode()
		default:
			log.Printf("WARNING: Ignoring invalid CallOpt passed to DeleteKey: %v\n", v)
		}
	}

	req.Header.Set("Prefer", preferHeaders[prefer])

	keys := Keys{}
	_, err = c.do(ctx, req, &keys)
	if err != nil {
		return nil, err
	}

	if len(keys.Keys) > 0 {
		return &keys.Keys[0], nil
	}

	return nil, nil
}

// Purge key method shreds all the metadata and registrations associated with a key that has been
// deleted. The purge operation is allowed to be performed on a key from 4 hours after its deletion
// and its action is irreversible.
// For more information please refer to the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-delete-keys#delete-keys-key-purge
func (c *Client) PurgeKey(ctx context.Context, idOrAlias string, prefer PreferReturn) (*Key, error) {
	req, err := c.newRequest("DELETE", fmt.Sprintf("keys/%s/purge", idOrAlias), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Prefer", preferHeaders[prefer])

	keys := Keys{}
	_, err = c.do(ctx, req, &keys)
	if err != nil {
		return nil, err
	}
	if len(keys.Keys) > 0 {
		return &keys.Keys[0], nil
	}

	return nil, nil
}

// RestoreKey method reverts a delete key status to active key
// This method performs restore of any key from deleted state to active state.
// For more information please refer to the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-restore-keys
func (c *Client) RestoreKey(ctx context.Context, idOrAlias string) (*Key, error) {
	req, err := c.newRequest("POST", fmt.Sprintf("keys/%s/restore", idOrAlias), nil)
	if err != nil {
		return nil, err
	}

	keysResponse := Keys{}

	_, err = c.do(ctx, req, &keysResponse)
	if err != nil {
		return nil, err
	}

	return &keysResponse.Keys[0], nil
}

// Wrap calls the wrap action with the given plain text.
func (c *Client) Wrap(ctx context.Context, idOrAlias string, plainText []byte, additionalAuthData *[]string) ([]byte, error) {
	_, ct, err := c.wrap(ctx, idOrAlias, plainText, additionalAuthData)
	return ct, err
}

// WrapCreateDEK calls the wrap action without plain text.
func (c *Client) WrapCreateDEK(ctx context.Context, idOrAlias string, additionalAuthData *[]string) ([]byte, []byte, error) {
	return c.wrap(ctx, idOrAlias, nil, additionalAuthData)
}

func (c *Client) wrap(ctx context.Context, idOrAlias string, plainText []byte, additionalAuthData *[]string) ([]byte, []byte, error) {
	keysActionReq := &KeysActionRequest{}

	if plainText != nil {
		_, err := base64.StdEncoding.DecodeString(string(plainText))
		if err != nil {
			return nil, nil, err
		}
		keysActionReq.PlainText = string(plainText)
	}

	if additionalAuthData != nil {
		keysActionReq.AAD = *additionalAuthData
	}

	keysAction, err := c.doKeysAction(ctx, idOrAlias, "wrap", keysActionReq)
	if err != nil {
		return nil, nil, err
	}

	pt := []byte(keysAction.PlainText)
	ct := []byte(keysAction.CipherText)

	return pt, ct, nil
}

// WrapCreateDEKV2 supports KeyVersion details with DEK and WrapDEK
func (c *Client) WrapCreateDEKV2(ctx context.Context, idOrAlias string, additionalAuthData *[]string) (*KeyActionResponse, error) {
	return c.WrapV2(ctx, idOrAlias, nil, additionalAuthData)
}

// WrapWithKeyVersion function supports KeyVersion Details, PlainText and Cyphertext in response
func (c *Client) WrapV2(ctx context.Context, idOrAlias string, plainText []byte, additionalAuthData *[]string) (*KeyActionResponse, error) {
	keysActionReq := &KeysActionRequest{}
	keyActionRes := &KeyActionResponse{}

	if plainText != nil {
		_, err := base64.StdEncoding.DecodeString(string(plainText))
		if err != nil {
			return keyActionRes, err
		}
		keysActionReq.PlainText = string(plainText)
	}

	if additionalAuthData != nil {
		keysActionReq.AAD = *additionalAuthData
	}

	keysAction, err := c.doKeysAction(ctx, idOrAlias, "wrap", keysActionReq)
	if err != nil {
		return keyActionRes, err
	}

	keyActionRes = &KeyActionResponse{
		PlainText:  keysAction.PlainText,
		CipherText: keysAction.CipherText,
	}
	if keysAction.KeyVersion != nil {
		keyActionRes.KeyVersion = &KeyVersion{
			ID: keysAction.KeyVersion.ID,
		}
		if keysAction.KeyVersion.CreationDate != nil {
			keyActionRes.KeyVersion.CreationDate = keysAction.KeyVersion.CreationDate
		}
	}
	return keyActionRes, nil
}

// Unwrap is deprecated since it returns only plaintext and doesn't know how to handle rotation.
func (c *Client) Unwrap(ctx context.Context, idOrAlias string, cipherText []byte, additionalAuthData *[]string) ([]byte, error) {
	plainText, _, err := c.UnwrapV2(ctx, idOrAlias, cipherText, additionalAuthData)
	if err != nil {
		return nil, err
	}
	return plainText, nil
}

// Unwrap with rotation support.
func (c *Client) UnwrapV2(ctx context.Context, idOrAlias string, cipherText []byte, additionalAuthData *[]string) ([]byte, []byte, error) {

	keysAction := &KeysActionRequest{
		CipherText: string(cipherText),
	}

	if additionalAuthData != nil {
		keysAction.AAD = *additionalAuthData
	}

	respAction, err := c.doKeysAction(ctx, idOrAlias, "unwrap", keysAction)
	if err != nil {
		return nil, nil, err
	}

	plainText := []byte(respAction.PlainText)
	rewrapped := []byte(respAction.CipherText)

	return plainText, rewrapped, nil
}

// Rotate rotates a CRK.
func (c *Client) Rotate(ctx context.Context, idOrAlias, payload string) error {

	actionReq := &KeysActionRequest{
		Payload: payload,
	}

	_, err := c.doKeysAction(ctx, idOrAlias, "rotate", actionReq)
	if err != nil {
		return err
	}

	return nil
}

type KeyPayload struct {
	payload             string
	encryptedNonce      string
	iv                  string
	encryptionAlgorithm string
}

func NewKeyPayload(payload, encryptedNonce, iv string) KeyPayload {
	kp := KeyPayload{
		payload:        payload,
		encryptedNonce: encryptedNonce,
		iv:             iv,
	}
	return kp
}

// EncryptWithRSA256 sets the encryption algorithm for key create to RSAES_OAEP_SHA_256
// This is the default algorithm for key creation under Key Protect service
func (kp KeyPayload) WithRSA256() KeyPayload {
	kp.encryptionAlgorithm = "RSAES_OAEP_SHA_256"
	return kp
}

// EncryptWithRSA1 sets the encryption algorithm for key create to RSAES_OAEP_SHA_1
// This algorithm is only supported by the Hyper Protect(HPCS) service
func (kp KeyPayload) WithRSA1() KeyPayload {
	kp.encryptionAlgorithm = "RSAES_OAEP_SHA_1"
	return kp
}

// RotateV2 methods supports rotation of a root key with or without payload and also rotate a
// securely imported root key.
func (c *Client) RotateV2(ctx context.Context, idOrAlias string, new_key *KeyPayload) error {
	var actionReq *KeysActionRequest
	if new_key != nil {
		actionReq = &KeysActionRequest{
			Payload:             new_key.payload,
			EncryptedNonce:      new_key.encryptedNonce,
			IV:                  new_key.iv,
			EncryptionAlgorithm: new_key.encryptionAlgorithm,
		}
	}

	_, err := c.doKeysAction(ctx, idOrAlias, "rotate", actionReq)
	if err != nil {
		return err
	}

	return nil
}

// SyncAssociatedResources method executes the sync request which verifies and updates
// the resources associated with the key.
// For more information please refer to the link below
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-sync-associated-resources
func (c *Client) SyncAssociatedResources(ctx context.Context, idOrAlias string) error {
	req, err := c.newRequest("POST", fmt.Sprintf("keys/%s/actions/sync", idOrAlias), nil)
	if err != nil {
		return err
	}

	_, err = c.do(ctx, req, nil)

	return err
}

// Disable a key. The key will not be deleted but it will not be active
// and key operations cannot be performed on a disabled key.
// For more information can refer to the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-disable-keys
func (c *Client) DisableKey(ctx context.Context, idOrAlias string) error {
	_, err := c.doKeysAction(ctx, idOrAlias, "disable", nil)
	return err
}

// Enable a key. Only disabled keys can be enabled. After enable
// the key becomes active and key operations can be performed on it.
// Note: This does not recover Deleted keys.
// For more information can refer to the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-disable-keys#enable-api
func (c *Client) EnableKey(ctx context.Context, idOrAlias string) error {
	_, err := c.doKeysAction(ctx, idOrAlias, "enable", nil)
	return err
}

// InitiateDualAuthDelete sets a key for deletion. The key must be configured with a DualAuthDelete policy.
// After the key is set to deletion it can be deleted by another user who has Manager access.
// For more information refer to the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-delete-dual-auth-keys#set-key-deletion-api
func (c *Client) InitiateDualAuthDelete(ctx context.Context, idOrAlias string) error {
	_, err := c.doKeysAction(ctx, idOrAlias, "setKeyForDeletion", nil)
	return err
}

// CancelDualAuthDelete unsets the key for deletion. If a key is set for deletion, it can
// be prevented from getting deleted by unsetting the key for deletion.
// For more information refer to the Key Protect docs in the link below:
// https://cloud.ibm.com/docs/key-protect?topic=key-protect-delete-dual-auth-keys#unset-key-deletion-api
func (c *Client) CancelDualAuthDelete(ctx context.Context, idOrAlias string) error {
	_, err := c.doKeysAction(ctx, idOrAlias, "unsetKeyForDeletion", nil)
	return err
}

// doKeysAction calls the KP Client to perform an action on a key.
func (c *Client) doKeysAction(ctx context.Context, idOrAlias string, action string, keysActionReq *KeysActionRequest) (*KeysActionRequest, error) {
	keyActionRsp := KeysActionRequest{}

	req, err := c.newRequest("POST", fmt.Sprintf("keys/%s/actions/%s", idOrAlias, action), keysActionReq)
	if err != nil {
		return nil, err
	}

	_, err = c.do(ctx, req, &keyActionRsp)
	if err != nil {
		return nil, err
	}
	return &keyActionRsp, nil
}
