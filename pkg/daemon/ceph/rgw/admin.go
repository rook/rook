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

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
)

type Context struct {
	context     *clusterd.Context
	Name        string
	ClusterName string
}

func NewContext(context *clusterd.Context, name, clusterName string) *Context {
	return &Context{context: context, Name: name, ClusterName: clusterName}
}

func runAdminCommandNoRealm(c *Context, args ...string) (string, error) {
	command, args := client.FinalizeCephCommandArgs("radosgw-admin", args, c.context.ConfigDir, c.ClusterName)

	// start the rgw admin command
	output, err := c.context.Executor.ExecuteCommandWithOutput(false, "", command, args...)
	if err != nil {
		return "", fmt.Errorf("failed to run radosgw-admin: %+v", err)
	}

	return output, nil
}

func runAdminCommand(c *Context, args ...string) (string, error) {
	options := []string{
		fmt.Sprintf("--rgw-realm=%s", c.Name),
		fmt.Sprintf("--rgw-zonegroup=%s", c.Name),
	}
	return runAdminCommandNoRealm(c, append(args, options...)...)
}
