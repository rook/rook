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

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
)

// Context holds the context for the object store.
type Context struct {
	Context     *clusterd.Context
	Name        string
	ClusterName string
	RunAsUser   string
}

// NewContext creates a new object store context.
func NewContext(context *clusterd.Context, name, clusterName string) *Context {
	return &Context{Context: context, Name: name, ClusterName: clusterName, RunAsUser: client.AdminUsername}
}

func RunAdminCommandNoRealm(c *Context, args ...string) (string, error) {
	command, args := client.FinalizeCephCommandArgs("radosgw-admin", args, c.Context.ConfigDir, c.ClusterName, c.RunAsUser)

	// start the rgw admin command
	output, err := c.Context.Executor.ExecuteCommandWithOutput(command, args...)
	if err != nil {
		return output, err
	}

	return output, nil
}

func runAdminCommand(c *Context, args ...string) (string, error) {
	// If the objectStoreName is not passed in the storage class
	// This means we are pointing to an external cluster so these commands are not needed
	// simply because the external cluster mode does not support that yet
	if c.Name != "" {
		options := []string{
			fmt.Sprintf("--rgw-realm=%s", c.Name),
			fmt.Sprintf("--rgw-zonegroup=%s", c.Name),
		}

		return RunAdminCommandNoRealm(c, append(args, options...)...)
	}

	return RunAdminCommandNoRealm(c, args...)
}
