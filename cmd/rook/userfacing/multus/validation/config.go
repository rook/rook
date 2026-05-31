/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package validation

import (
	"fmt"

	"github.com/rook/rook/pkg/daemon/multus"
	"github.com/spf13/cobra"
)

var (
	configCmd = &cobra.Command{
		Use:   "config",
		Short: "Generate a validation test config file for different default scenarios to stdout.",
		Args:  cobra.NoArgs,
	}

	converged = &cobra.Command{
		Use:   "converged",
		Short: "Example config for a cluster that runs storage and user workloads on all nodes.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			y, err := multus.NewDefaultValidationTestConfig().ToYAML()
			if err != nil {
				return err
			}
			fmt.Print(y)
			return nil
		},
	}

	dedicatedStorageNodesConfigCmd = &cobra.Command{
		Use:   "dedicated-storage-nodes",
		Short: "Example config file for a cluster that uses dedicated storage nodes.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			y, err := multus.NewDedicatedStorageNodesValidationTestConfig().ToYAML()
			if err != nil {
				return err
			}
			fmt.Print(y)
			return nil
		},
	}

	stretchClusterConfigCmd = &cobra.Command{
		Use:   "stretch-cluster",
		Short: "Example config file for a stretch cluster with dedicated storage nodes.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			y, err := multus.NewArbiterValidationTestConfig().ToYAML()
			if err != nil {
				return err
			}
			fmt.Print(y)
			return nil
		},
	}
)

func init() {
	configCmd.AddCommand(converged)
	configCmd.AddCommand(dedicatedStorageNodesConfigCmd)
	configCmd.AddCommand(stretchClusterConfigCmd)
}
