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
package mgr

import (
	"fmt"

	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
)

func getKeyringProperties(name string) (string, []string) {
	username := fmt.Sprintf("mgr.%s", name)
	access := []string{"mon", "allow *"}
	return username, access
}

// create a keyring for the mds client with a limited set of privileges
func CreateKeyring(context *clusterd.Context, clusterName, name string) (string, error) {
	// get-or-create-key for the user account
	username, access := getKeyringProperties(name)
	keyring, err := client.AuthGetOrCreateKey(context, clusterName, username, access)
	if err != nil {
		return "", fmt.Errorf("failed to get or create auth key for %s. %+v", username, err)
	}

	return keyring, nil
}
