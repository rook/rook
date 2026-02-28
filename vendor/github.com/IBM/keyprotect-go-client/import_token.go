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
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"hash"
	"io"
	"time"
)

// EncryptionAlgorithm represents the encryption algorithm used for key creation
const (
	// AlgorithmRSAOAEP256 denotes RSA OAEP SHA 256 encryption, supported by KP
	AlgorithmRSAOAEP256 string = "RSAES_OAEP_SHA_256"
	// AlgorithmRSAOAEP1 denotes RSA OAEP SHA 1 encryption, supported by HPCS
	AlgorithmRSAOAEP1 string = "RSAES_OAEP_SHA_1"
)

// ImportTokenCreateRequest represents request parameters for creating a
// ImportToken.
type ImportTokenCreateRequest struct {
	MaxAllowedRetrievals int `json:"maxAllowedRetrievals,omitempty"`
	ExpiresInSeconds     int `json:"expiration,omitempty"`
}

// ImportTokenKeyResponse represents the response body for various ImportToken
// API calls.
type ImportTokenKeyResponse struct {
	ID             string     `json:"id"`
	CreationDate   *time.Time `json:"creationDate"`
	ExpirationDate *time.Time `json:"expirationDate"`
	Payload        string     `json:"payload"`
	Nonce          string     `json:"nonce"`
}

// ImportTokenMetadata represents the metadata of a ImportToken.
type ImportTokenMetadata struct {
	ID                   string     `json:"id"`
	CreationDate         *time.Time `json:"creationDate"`
	ExpirationDate       *time.Time `json:"expirationDate"`
	MaxAllowedRetrievals int        `json:"maxAllowedRetrievals"`
	RemainingRetrievals  int        `json:"remainingRetrievals"`
}

// CreateImportToken creates a key ImportToken.
func (c *Client) CreateImportToken(ctx context.Context, expiration, maxAllowedRetrievals int) (*ImportTokenMetadata, error) {
	reqBody := ImportTokenCreateRequest{
		MaxAllowedRetrievals: maxAllowedRetrievals,
		ExpiresInSeconds:     expiration,
	}

	req, err := c.newRequest("POST", "import_token", &reqBody)
	if err != nil {
		return nil, err
	}

	res := ImportTokenMetadata{}
	if _, err := c.do(ctx, req, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

// GetImportTokenTransportKey retrieves the ImportToken transport key.
func (c *Client) GetImportTokenTransportKey(ctx context.Context) (*ImportTokenKeyResponse, error) {
	res := ImportTokenKeyResponse{}

	req, err := c.newRequest("GET", "import_token", nil)
	if err != nil {
		return nil, err
	}

	if _, err := c.do(ctx, req, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

// EncryptNonce will wrap the KP generated nonce with the users key-material
func EncryptNonce(key, value, iv string) (string, string, error) {
	return encryptNonce(key, value, iv)
}

// EncryptKey will encrypt the user key-material with the public key from key protect
func EncryptKey(key, pubkey string) (string, error) {
	return encryptKey(key, pubkey)
}

func encryptNonce(key, value, iv string) (string, string, error) {
	var cipherText []byte
	pubKey, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", "", fmt.Errorf("Failed to decode public key: %s", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", "", fmt.Errorf("Failed to decode nonce: %s", err)
	}
	block, err := aes.NewCipher(pubKey)
	if err != nil {
		return "", "", err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	if iv == "" {
		newIv := make([]byte, 12)
		if _, err := io.ReadFull(rand.Reader, newIv); err != nil {
			panic(err.Error())
		}
		cipherText = aesgcm.Seal(nil, newIv, nonce, nil)
		return base64.StdEncoding.EncodeToString(cipherText), base64.StdEncoding.EncodeToString(newIv), nil
	}
	cipherText = aesgcm.Seal(nil, []byte(iv), nonce, nil)
	return base64.StdEncoding.EncodeToString(cipherText), iv, nil
}

// EncryptNonceWithCBCPAD encrypts the nonce using the user's key-material
// with CBC encrypter. It will also pad the nonce using pkcs7. This is needed
// for Hyper Protect Crypto Services, since it supports only CBC Encryption.
func EncryptNonceWithCBCPAD(key, value, iv string) (string, string, error) {
	keyMat, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", "", fmt.Errorf("Failed to decode Key: %s", err)
	}

	nonce, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", "", fmt.Errorf("Failed to decode Nonce: %s", err)
	}

	block, err := aes.NewCipher(keyMat)
	if err != nil {
		return "", "", err
	}

	// PKCS7 Padding
	paddingLength := aes.BlockSize - (len(nonce) % aes.BlockSize)
	paddingBytes := []byte{byte(paddingLength)}
	paddingText := bytes.Repeat(paddingBytes, paddingLength)
	nonce = append(nonce, paddingText...)

	var newIv []byte
	if iv != "" {
		newIv = []byte(iv)
	} else {
		newIv = make([]byte, aes.BlockSize)
		// Generate an IV to achieve semantic security
		if _, err := io.ReadFull(rand.Reader, newIv); err != nil {
			return "", "", fmt.Errorf("Failed to generate IV: %s", err)
		}
	}

	cipherText := make([]byte, len(nonce))

	mode := cipher.NewCBCEncrypter(block, newIv)
	mode.CryptBlocks(cipherText, nonce)

	return base64.StdEncoding.EncodeToString(cipherText), base64.StdEncoding.EncodeToString(newIv), nil
}

// encryptKey uses sha256 to encrypt the key
func encryptKey(key, pubKey string) (string, error) {
	return encryptKeyWithSHA(key, pubKey, sha256.New())
}

// EncryptKeyWithSHA1 uses sha1 to encrypt the key
func EncryptKeyWithSHA1(key, pubKey string) (string, error) {
	return encryptKeyWithSHA(key, pubKey, sha1.New())
}

func encryptKeyWithSHA(key, pubKey string, sha hash.Hash) (string, error) {
	decodedPubKey, err := base64.StdEncoding.DecodeString(pubKey)
	if err != nil {
		return "", fmt.Errorf("Failed to decode public key: %s", err)
	}
	keyMat, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", fmt.Errorf("Failed to decode key material: %s", err)
	}
	pubKeyBlock, _ := pem.Decode(decodedPubKey)
	if pubKeyBlock == nil {
		return "", fmt.Errorf("Failed to decode public key into pem format: %s", err)
	}
	parsedPubKey, err := x509.ParsePKIXPublicKey(pubKeyBlock.Bytes)
	if err != nil {
		return "", fmt.Errorf("Failed to parse public key: %s", err)
	}
	publicKey, isRSAPublicKey := parsedPubKey.(*rsa.PublicKey)
	if !isRSAPublicKey {
		return "", fmt.Errorf("invalid public key")
	}
	encryptedKey, err := rsa.EncryptOAEP(sha, rand.Reader, publicKey, keyMat, []byte(""))
	if err != nil {
		return "", fmt.Errorf("Failed to encrypt key: %s", err)
	}
	return base64.StdEncoding.EncodeToString(encryptedKey), nil
}
