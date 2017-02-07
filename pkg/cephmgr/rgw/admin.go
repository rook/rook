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

	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
)

// Runs the embedded radosgw-admin command and returns its output
func RunAdminCommand(context *clusterd.Context, command, subcommand string, args ...string) (string, error) {
	cluster, err := mon.LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return "", fmt.Errorf("failed to get cluster name. %+v", err)
	}

	return RunAdminCommandWithClusterInfo(context, cluster, command, subcommand, args...)
}

func RunAdminCommandWithClusterInfo(context *clusterd.Context, cluster *mon.ClusterInfo, command, subcommand string, args ...string) (string, error) {
	confFile, keyringFile, _, err := mon.GenerateTempConfigFiles(context, cluster)
	if err != nil {
		return "", fmt.Errorf("failed to generate config file. %+v", err)
	}

	options := []string{
		command,
		subcommand,
		fmt.Sprintf("--cluster=%s", cluster.Name),
		fmt.Sprintf("--conf=%s", confFile),
		fmt.Sprintf("--keyring=%s", keyringFile),
	}
	options = append(options, args...)

	// start the rgw admin command
	output, err := context.ProcMan.RunWithCombinedOutput("rgw-admin", "rgw-admin", options...)
	if err != nil {
		return "", fmt.Errorf("failed to run rgw-admin: %+v", err)
	}

	return output, nil
}
