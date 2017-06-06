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

func AuthGetKey(context *clusterd.Context, clusterName, name string) (string, error) {
	args := []string{"auth", "get-key", name}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return "", fmt.Errorf("failed to get key for %s: %+v", name, err)
	}

	return parseAuthKey(buf)
}

func AuthGetOrCreateKey(context *clusterd.Context, clusterName, name string, caps []string) (string, error) {

	args := append([]string{"auth", "get-or-create-key", name}, caps...)
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return "", fmt.Errorf("failed get-or-create-key %s: %+v", name, err)
	}

	logger.Infof("Parsing key: %v", buf)
	return parseAuthKey(buf)
}

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
