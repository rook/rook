/*
Copyright 2017 The Rook Authors. All rights reserved.

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

package cmd

import (
	"encoding/json"
	"os"
	"strconv"

	"github.com/rook/rook/pkg/operator/ceph/agent"
	"github.com/spf13/cobra"
	"k8s.io/kubernetes/pkg/volume/flexvolume"
)

var (
	initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize the volume plugin",
		RunE:  initPlugin,
	}
)

func init() {
	RootCmd.AddCommand(initCmd)
}

func initPlugin(cmd *cobra.Command, args []string) error {
	rookEnableSelinuxRelabeling, err := strconv.ParseBool(os.Getenv(agent.RookEnableSelinuxRelabelingEnv))
	if err != nil {
		// Don't log any errors to stdout as this will break the init. Just default the value to true.
		rookEnableSelinuxRelabeling = true
	}

	rookEnableFSGroup, err := strconv.ParseBool(os.Getenv(agent.RookEnableFSGroupEnv))
	if err != nil {
		// Don't log any errors to stdout as this will break the init. Just default the value to true.
		rookEnableFSGroup = true
	}

	status := DriverStatus{
		Status: flexvolume.StatusSuccess,
		Capabilities: &DriverCapabilities{
			Attach: false,
			// Required for any mount peformed on a host running selinux
			SELinuxRelabel: rookEnableSelinuxRelabeling,
			FSGroup:        rookEnableFSGroup,
		},
	}
	if err := json.NewEncoder(os.Stdout).Encode(&status); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

// FIX: After we move to the k8s 1.13 dependencies, we can remove this type
// and reference the type in k8s.io/kubernetes/pkg/volume/flexvolume/driver-call.go
type DriverStatus struct {
	// Status of the callout. One of "Success", "Failure" or "Not supported".
	Status string `json:"status"`
	// Reason for success/failure.
	Message string `json:"message,omitempty"`
	// Path to the device attached. This field is valid only for attach calls.
	// ie: /dev/sdx
	DevicePath string `json:"device,omitempty"`
	// Cluster wide unique name of the volume.
	VolumeName string `json:"volumeName,omitempty"`
	// Represents volume is attached on the node
	Attached bool `json:"attached,omitempty"`
	// Returns capabilities of the driver.
	// By default we assume all the capabilities are supported.
	// If the plugin does not support a capability, it can return false for that capability.
	Capabilities *DriverCapabilities `json:",omitempty"`
	// Returns the actual size of the volume after resizing is done, the size is in bytes.
	ActualVolumeSize int64 `json:"volumeNewSize,omitempty"`
}

// FIX: After we move to the k8s 1.13 dependencies, we can remove this type
// and reference the type in k8s.io/kubernetes/pkg/volume/flexvolume/driver-call.go
type DriverCapabilities struct {
	Attach           bool `json:"attach"`
	SELinuxRelabel   bool `json:"selinuxRelabel"`
	SupportsMetrics  bool `json:"supportsMetrics"`
	FSGroup          bool `json:"fsGroup"`
	RequiresFSResize bool `json:"requiresFSResize"`
}
