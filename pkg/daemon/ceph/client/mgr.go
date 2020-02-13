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
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
)

var (
	moduleEnableWaitTime = 5 * time.Second
)

// MgrEnableModule enables a mgr module
func MgrEnableModule(context *clusterd.Context, clusterName, name string, force bool) error {
	retryCount := 5
	var err error
	for i := 0; i < retryCount; i++ {
		if name == "balancer" {
			err = enableDisableBalancerModule(context, clusterName, "on")
		} else {
			err = enableModule(context, clusterName, name, force, "enable")
		}
		if err != nil {
			if i < retryCount-1 {
				logger.Warningf("failed to enable mgr module %q. trying again...", name)
				time.Sleep(moduleEnableWaitTime)
				continue
			} else {
				return errors.Wrapf(err, "failed to enable mgr module %q even after %d retries", name, retryCount)
			}
		}
		break
	}
	return nil
}

// MgrDisableModule disables a mgr module
func MgrDisableModule(context *clusterd.Context, clusterName, name string) error {
	if name == "balancer" {
		return enableDisableBalancerModule(context, clusterName, "off")
	}
	return enableModule(context, clusterName, name, false, "disable")
}

// MgrSetConfig applies a setting for a single mgr daemon
func MgrSetConfig(context *clusterd.Context, clusterName, mgrName string, key, val string, force bool) (bool, error) {
	var getArgs, setArgs []string
	mgrID := fmt.Sprintf("mgr.%s", mgrName)
	getArgs = append(getArgs, "config", "get", mgrID, key)
	if val == "" {
		setArgs = append(setArgs, "config", "rm", mgrID, key)
	} else {
		setArgs = append(setArgs, "config", "set", mgrID, key, val)
	}
	if force {
		setArgs = append(setArgs, "--force")
	}

	// Retrieve previous value to monitor changes
	var prevVal string
	buf, err := NewCephCommand(context, clusterName, getArgs).Run()
	if err == nil {
		prevVal = strings.TrimSpace(string(buf))
	}

	if _, err := NewCephCommand(context, clusterName, setArgs).Run(); err != nil {
		return false, errors.Wrapf(err, "failed to set mgr config key %s to \"%s\"", key, val)
	}

	hasChanged := prevVal != val
	return hasChanged, nil
}

func enableModule(context *clusterd.Context, clusterName, name string, force bool, action string) error {
	args := []string{"mgr", "module", action, name}
	if force {
		args = append(args, "--force")
	}

	_, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to enable mgr module %q", name)
	}

	return nil
}

// enableDisableBalancerModule enables the ceph balancer module
func enableDisableBalancerModule(context *clusterd.Context, clusterName, action string) error {
	args := []string{"balancer", action}
	_, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to turn %q the balancer module", action)
	}

	return nil
}

func setBalancerMode(context *clusterd.Context, clusterName, mode string) error {
	args := []string{"balancer", "mode", mode}
	_, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set balancer mode %q", mode)
	}

	return nil
}

// SetMinCompatClientLuminous set the minimum compatibility for clients to Luminous
func SetMinCompatClientLuminous(context *clusterd.Context, clusterName string) error {
	args := []string{"osd", "set-require-min-compat-client", "luminous", "--yes-i-really-mean-it"}
	_, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrap(err, "failed to set set-require-min-compat-client to luminous")
	}

	return nil
}

// MgrSetBalancerMode sets the given mode to the balancer module
func MgrSetBalancerMode(context *clusterd.Context, clusterName, balancerModuleMode string) error {
	retryCount := 5
	for i := 0; i < retryCount; i++ {
		err := setBalancerMode(context, clusterName, balancerModuleMode)
		if err != nil {
			if i < retryCount-1 {
				logger.Warningf("failed to set mgr module mode %q. trying again...", balancerModuleMode)
				time.Sleep(moduleEnableWaitTime)
				continue
			} else {
				return errors.Wrapf(err, "failed to set mgr module mode %q even after %d retries", balancerModuleMode, retryCount)
			}
		}
		break
	}

	return nil
}
