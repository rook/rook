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
package client

import (
	"encoding/json"
	"fmt"

	"github.com/rook/rook/pkg/clusterd"
)

// AuthAdd will create a new user with the given capabilities and using the already generated keyring
// found at the given keyring path.  This should not be used when the user may already exist.
func AuthAdd(context *clusterd.Context, clusterName, name, keyringPath string, caps []string) error {
	args := append([]string{"auth", "add", name, "-i", keyringPath}, caps...)
	_, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to auth add for %s: %+v", name, err)
	}

	return nil
}

// AuthGetOrCreate will either get or create a user with the given capabilities.  The keyring for the
// user will be written to the given keyring path.
func AuthGetOrCreate(context *clusterd.Context, clusterName, name, keyringPath string, caps []string) error {
	args := append([]string{"auth", "get-or-create", name, "-o", keyringPath}, caps...)
	_, err := ExecuteCephCommandPlainNoOutputFile(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to auth get-or-create for %s: %+v", name, err)
	}

	return nil
}

// AuthGetKey gets the key for the given user.
func AuthGetKey(context *clusterd.Context, clusterName, name string) (string, error) {
	args := []string{"auth", "get-key", name}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return "", fmt.Errorf("failed to get key for %s: %+v", name, err)
	}

	return parseAuthKey(buf)
}

// AuthGetOrCreateKey gets or creates the key for the given user.
func AuthGetOrCreateKey(context *clusterd.Context, clusterName, name string, caps []string) (string, error) {
	args := append([]string{"auth", "get-or-create-key", name}, caps...)
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return "", fmt.Errorf("failed get-or-create-key %s: %+v", name, err)
	}

	return parseAuthKey(buf)
}

// AuthUpdateCaps updates the capabilities for the given user.
func AuthUpdateCaps(context *clusterd.Context, clusterName, name string, caps []string) error {
	args := append([]string{"auth", "caps", name}, caps...)
	_, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to update caps for %s. %+v", name, err)
	}
	return err
}

// AuthDelete will delete the given user.
func AuthDelete(context *clusterd.Context, clusterName, name string) error {
	args := []string{"auth", "del", name}
	_, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to delete auth for %s. %v", name, err)
	}
	return nil
}

func parseAuthKey(buf []byte) (string, error) {
	var resp map[string]interface{}
	if err := json.Unmarshal(buf, &resp); err != nil {
		return "", fmt.Errorf("failed to unmarshal get/create key response: %+v", err)
	}
	return resp["key"].(string), nil
}
