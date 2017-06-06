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
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
)

func RunAdminCommand(context *clusterd.Context, getClusterInfo func() (*mon.ClusterInfo, error), command, subcommand string, args ...string) (string, error) {
	cluster, err := getClusterInfo()
	if err != nil {
		return "", fmt.Errorf("failed to get cluster info. %+v", err)
	}

	options := []string{
		command,
		subcommand,
	}
	options = client.AppendAdminConnectionArgs(options, context.ConfigDir, cluster.Name)

	// start the rgw admin command
	output, err := context.Executor.ExecuteCommandWithOutput("", "radosgw-admin", options...)
	if err != nil {
		return "", fmt.Errorf("failed to run radosgw-admin: %+v", err)
	}

	return output, nil
}
