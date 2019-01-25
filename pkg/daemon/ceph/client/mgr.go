/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package client

import (
	"fmt"
	"strings"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
)

// MgrEnableModule enables a mgr module
func MgrEnableModule(context *clusterd.Context, clusterName, name string, force bool) error {
	return enableModule(context, clusterName, name, force, "enable")
}

// MgrDisableModule disables a mgr module
func MgrDisableModule(context *clusterd.Context, clusterName, name string) error {
	return enableModule(context, clusterName, name, false, "disable")
}

// MgrSetAllConfig applies a setting for all mgr daemons
func MgrSetAllConfig(context *clusterd.Context, clusterName, cephVersionName, key, val string) (bool, error) {
	return MgrSetConfig(context, clusterName, "", cephVersionName, key, val, false)
}

// MgrSetConfig applies a setting for a single mgr daemon
func MgrSetConfig(context *clusterd.Context, clusterName, mgrName, cephVersionName, key, val string, force bool) (bool, error) {
	var getArgs, setArgs []string
	mgrID := fmt.Sprintf("mgr.%s", mgrName)
	if cephVersionName == cephv1.Luminous || cephVersionName == "" {
		getArgs = append(getArgs, "config-key", "get", key)
		if val == "" {
			setArgs = append(setArgs, "config-key", "del", key)
		} else {
			setArgs = append(setArgs, "config-key", "set", key, val)
		}
	} else {
		getArgs = append(getArgs, "config", "get", mgrID, key)
		if val == "" {
			setArgs = append(setArgs, "config", "rm", mgrID, key)
		} else {
			setArgs = append(setArgs, "config", "set", mgrID, key, val)
		}
		if force && cephv1.VersionAtLeast(cephVersionName, cephv1.Nautilus) {
			setArgs = append(setArgs, "--force")
		}
	}

	// Retrieve previous value to monitor changes
	var prevVal string
	buf, err := ExecuteCephCommand(context, clusterName, getArgs)
	if err == nil {
		prevVal = strings.TrimSpace(string(buf))
	}

	if _, err := ExecuteCephCommand(context, clusterName, setArgs); err != nil {
		return false, fmt.Errorf("failed to set mgr config key %s to \"%s\": %+v", key, val, err)
	}

	hasChanged := prevVal != val
	return hasChanged, nil
}

func enableModule(context *clusterd.Context, clusterName, name string, force bool, action string) error {
	args := []string{"mgr", "module", action, name}
	if force {
		args = append(args, "--force")
	}
	_, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to mgr module enable for %s: %+v", name, err)
	}

	return nil
}
