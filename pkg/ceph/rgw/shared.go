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
package rgw

import (
	"fmt"

	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
)

// create a keyring for the rgw client with a limited set of privileges
func CreateKeyring(context *clusterd.Context, clusterName string) (string, error) {
	username := "client.radosgw.gateway"
	access := []string{"osd", "allow rwx", "mon", "allow rw"}

	// get-or-create-key for the user account
	key, err := client.AuthGetOrCreateKey(context, clusterName, username, access)
	if err != nil {
		return "", fmt.Errorf("failed to get or create auth key for %s. %+v", username, err)
	}

	return key, err
}
