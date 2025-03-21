/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package util

import (
	"bytes"
	"fmt"
	"os"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	rook "github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/topology"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/spf13/cobra"
)

var NodeTopologyCmd = &cobra.Command{
	Use: "inject-node-topology",
}

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "utilcmd")
)

func init() {
	NodeTopologyCmd.RunE = SetNodeTopologyEnv
}

// SetNodeTopologyEnv fetches the node topology labels and saves them in a file
func SetNodeTopologyEnv(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	context := rook.NewContext()

	nodeName := os.Getenv(k8sutil.NodeNameEnvVar)

	if nodeName == "" {
		return fmt.Errorf("NODE_NAME env variable not found in the pod container")
	}

	nodeLabels, err := k8sutil.GetNodeLabels(ctx, context.Clientset, nodeName)
	if err != nil {
		return errors.Wrapf(err, "failed to get labels for node %q.", nodeName)
	}

	topologyLabels, _ := topology.ExtractOSDTopologyFromLabels(nodeLabels)

	topologyLabelsString := topologyLabelsKVPairs(topologyLabels)
	topologyLabelsString = fmt.Sprintf("export %s", topologyLabelsString)
	logger.Infof("node topology labels:  %q", topologyLabelsString)

	topologyLabelsFile, err := os.CreateTemp("/tmp", "env")
	if err != nil {
		return errors.Wrapf(err, "failed to create node label file")
	}
	err = os.WriteFile(topologyLabelsFile.Name(), []byte(topologyLabelsString), 0400)
	if err != nil {
		return errors.Wrapf(err, "failed to write node label envs to file %q", topologyLabelsFile.Name())
	}

	logger.Infof("successfully stored node topology labels in %q", topologyLabelsFile.Name())

	return nil
}

func topologyLabelsKVPairs(m map[string]string) string {
	b := new(bytes.Buffer)
	for k, v := range m {
		fmt.Fprintf(b, "%s=\"%s\" ", k, v)
	}
	return b.String()
}
