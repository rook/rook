/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/daemon/ceph/client"
)

func CreateConfigDir(configDir string) error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return errors.Wrap(err, "error while creating directory")
	}
	if err := ioutil.WriteFile(path.Join(configDir, "client.admin.keyring"), []byte("key = adminsecret"), 0600); err != nil {
		return errors.Wrap(err, "admin writefile error")
	}
	if err := ioutil.WriteFile(path.Join(configDir, "mon.keyring"), []byte("key = monsecret"), 0600); err != nil {
		return errors.Wrap(err, "mon writefile error")
	}
	return nil
}

// CreateTestClusterInfo creates a test cluster info
// This would be best in a test package, but is included here to avoid cyclic dependencies
func CreateTestClusterInfo(monCount int) *client.ClusterInfo {
	ownerInfo := client.NewMinimumOwnerInfoWithOwnerRef()
	c := &client.ClusterInfo{
		FSID:          "12345",
		Namespace:     "default",
		MonitorSecret: "monsecret",
		CephCred: client.CephCred{
			Username: client.AdminUsername,
			Secret:   "adminkey",
		},
		Monitors:  map[string]*client.MonInfo{},
		OwnerInfo: ownerInfo,
		Context:   context.TODO(),
	}
	mons := []string{"a", "b", "c", "d", "e"}
	for i := 0; i < monCount; i++ {
		id := mons[i]
		c.Monitors[id] = &client.MonInfo{
			Name:     id,
			Endpoint: fmt.Sprintf("1.2.3.%d:6789", (i + 1)),
		}
	}
	c.SetName(c.Namespace)
	return c
}
