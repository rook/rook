/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package secret

import (
	"encoding/json"
	"strings"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/pkg/errors"
)

const (
	//#nosec G101 -- This is the name of an annotation key, not a secret
	UpdateObjectUserSecretAnnotation = "rook.io/source-of-truth"
	SourceOfTruthSecret              = "secret"
)

// Credential is the user credential values
type Credential struct {
	AccessKey string `json:"access_key" url:"access-key"`
	SecretKey string `json:"secret_key" url:"secret-key"`
}

func ValidateCredentials(accessKey, secretKey string, credentials []Credential) ([]Credential, error) {
	// if len is 0 just update the credentials array with the accessKey and secretKey key
	if len(credentials) == 0 {
		credentials = []Credential{
			{AccessKey: accessKey,
				SecretKey: secretKey}}
		return credentials, nil
	}
	// match if the credentials array contains the accessKey and secretKey key
	for i := range credentials {
		if credentials[i].AccessKey == accessKey && credentials[0].SecretKey == secretKey {
			return credentials, nil
		}
	}

	return credentials, errors.Errorf("secret keys data is invalid, please update the secret with valid format")
}

func UnmarshalKeys(credentials []byte) ([]Credential, error) {
	var userKeys []Credential
	err := json.Unmarshal(credentials, &userKeys)
	if err != nil {
		return userKeys, errors.Wrapf(err, "unable to unmarshal credentials from the object secret")
	}
	return userKeys, nil
}

func GetUserDefinedSecretAnnotations(annotations map[string]string) bool {
	if value, found := annotations[UpdateObjectUserSecretAnnotation]; found {
		if strings.EqualFold(value, SourceOfTruthSecret) {
			return true
		}
	}
	return false
}

func UpdatecredentialsUserId(uid, displayName string, credentials []Credential) []admin.UserKeySpec {
	var updatedCredentials []admin.UserKeySpec
	for i := range credentials {
		userSpec := admin.UserKeySpec{User: displayName, UID: uid, AccessKey: credentials[i].AccessKey, SecretKey: credentials[i].SecretKey}
		updatedCredentials = append(updatedCredentials, userSpec)
	}

	return updatedCredentials
}
