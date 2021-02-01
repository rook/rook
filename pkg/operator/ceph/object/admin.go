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

package object

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
)

// Context holds the context for the object store.
type Context struct {
	Context     *clusterd.Context
	clusterInfo *client.ClusterInfo
	Name        string
	UID         string
	Endpoint    string
	Realm       string
	ZoneGroup   string
	Zone        string
}

// NewContext creates a new object store context.
func NewContext(context *clusterd.Context, clusterInfo *client.ClusterInfo, name string) *Context {
	return &Context{Context: context, Name: name, clusterInfo: clusterInfo}
}

func NewMultisiteContext(context *clusterd.Context, clusterInfo *client.ClusterInfo, store *cephv1.CephObjectStore) (*Context, error) {
	objContext := &Context{Context: context, Name: store.Name, clusterInfo: clusterInfo}
	realmName, zoneGroupName, zoneName, err := getMultisiteForObjectStore(context, store)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get realm/zone group/zone for object store %q", store.Name)
	}

	objContext.Realm = realmName
	objContext.ZoneGroup = zoneGroupName
	objContext.Zone = zoneName
	return objContext, nil
}

// RunAdminCommandNoMultisite is for running radosgw-admin commands in scenarios where an object-store has not been created yet or for commands on the realm or zonegroup (ex: radosgw-admin zonegroup get)
// This function times out after a fixed interval if no response is received.
func RunAdminCommandNoMultisite(c *Context, args ...string) (string, error) {
	command, args := client.FinalizeCephCommandArgs("radosgw-admin", c.clusterInfo, args, c.Context.ConfigDir)

	timeout, err := time.ParseDuration(fmt.Sprintf("%ss", client.CephConnectionTimeout))
	if err != nil {
		return "", errors.Wrap(err, "failed to parse CephConnectionTimeout")
	}

	// start the rgw admin command
	output, err := c.Context.Executor.ExecuteCommandWithTimeout(timeout, command, args...)
	if err != nil {
		return output, err
	}

	return output, nil
}

// This function is for running radosgw-admin commands in scenarios where an object-store has been created and the Context has been updated with the appropriate realm, zone group, and zone.
func runAdminCommand(c *Context, args ...string) (string, error) {
	// If the objectStoreName is not passed in the storage class
	// This means we are pointing to an external cluster so these commands are not needed
	// simply because the external cluster mode does not support that yet
	//
	// The following conditions tries to determine if the cluster is external
	// When connecting to an external cluster, the Ceph user is different than client.admin
	// This is not perfect though since "client.admin" is somehow supported...
	if c.Name != "" && c.clusterInfo.CephCred.Username == client.AdminUsername {
		options := []string{
			fmt.Sprintf("--rgw-realm=%s", c.Realm),
			fmt.Sprintf("--rgw-zonegroup=%s", c.ZoneGroup),
			fmt.Sprintf("--rgw-zone=%s", c.Zone),
		}

		return RunAdminCommandNoMultisite(c, append(args, options...)...)
	}

	return RunAdminCommandNoMultisite(c, args...)
}
