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
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
)

var (
	moduleEnableWaitTime = 5 * time.Second
)

func CephMgrMap(context *clusterd.Context, clusterInfo *ClusterInfo) (*MgrMap, error) {
	args := []string{"mgr", "dump"}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		if len(buf) > 0 {
			return nil, errors.Wrapf(err, "failed to get mgr dump. %s", string(buf))
		}
		return nil, errors.Wrap(err, "failed to get mgr dump")
	}

	var mgrMap MgrMap
	if err := json.Unmarshal([]byte(buf), &mgrMap); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal mgr dump")
	}

	return &mgrMap, nil
}

func CephMgrStat(context *clusterd.Context, clusterInfo *ClusterInfo) (*MgrStat, error) {
	args := []string{"mgr", "stat"}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		if len(buf) > 0 {
			return nil, errors.Wrapf(err, "failed to get mgr stat. %s", string(buf))
		}
		return nil, errors.Wrap(err, "failed to get mgr stat")
	}

	var mgrStat MgrStat
	if err := json.Unmarshal([]byte(buf), &mgrStat); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal mgr stat")
	}

	return &mgrStat, nil
}

// MgrEnableModule enables a mgr module
func MgrEnableModule(context *clusterd.Context, clusterInfo *ClusterInfo, name string, force bool) error {
	retryCount := 5
	var err error
	for i := 0; i < retryCount; i++ {
		/* In Pacific the balancer is now on by default in upmap mode.
		In earlier versions, the balancer was included in the ``always_on_modules`` list, but needed to be
		turned on explicitly using the ``ceph balancer on`` command. */
		if name == "balancer" {
			logger.Debug("balancer module is already 'on' on pacific, doing nothing", name)
			return nil
		} else {
			err = enableModule(context, clusterInfo, name, force, "enable")
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
func MgrDisableModule(context *clusterd.Context, clusterInfo *ClusterInfo, name string) error {
	if name == "balancer" {
		return enableDisableBalancerModule(context, clusterInfo, "off")
	}
	return enableModule(context, clusterInfo, name, false, "disable")
}

func enableModule(context *clusterd.Context, clusterInfo *ClusterInfo, name string, force bool, action string) error {
	args := []string{"mgr", "module", action, name}
	if force {
		args = append(args, "--force")
	}

	_, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to enable mgr module %q", name)
	}

	return nil
}

// enableDisableBalancerModule enables the ceph balancer module
func enableDisableBalancerModule(context *clusterd.Context, clusterInfo *ClusterInfo, action string) error {
	args := []string{"balancer", action}
	_, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to turn %q the balancer module", action)
	}

	return nil
}

func setBalancerMode(context *clusterd.Context, clusterInfo *ClusterInfo, mode string) error {
	args := []string{"balancer", "mode", mode}
	_, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set balancer mode %q", mode)
	}

	return nil
}

// setMinCompatClientLuminous set the minimum compatibility for clients to Luminous
func setMinCompatClientLuminous(context *clusterd.Context, clusterInfo *ClusterInfo) error {
	args := []string{"osd", "set-require-min-compat-client", "luminous", "--yes-i-really-mean-it"}
	_, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrap(err, "failed to set set-require-min-compat-client to luminous")
	}

	return nil
}

// mgrSetBalancerMode sets the given mode to the balancer module
func mgrSetBalancerMode(context *clusterd.Context, clusterInfo *ClusterInfo, balancerModuleMode string) error {
	retryCount := 5
	for i := 0; i < retryCount; i++ {
		err := setBalancerMode(context, clusterInfo, balancerModuleMode)
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

// ConfigureBalancerModule configures the balancer module
func ConfigureBalancerModule(context *clusterd.Context, clusterInfo *ClusterInfo, balancerModuleMode string) error {
	// Set min compat client to luminous before enabling the balancer mode "upmap"
	err := setMinCompatClientLuminous(context, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to set minimum compatibility client")
	}

	// Set balancer module mode
	err = mgrSetBalancerMode(context, clusterInfo, balancerModuleMode)
	if err != nil {
		return errors.Wrapf(err, "failed to set balancer module mode to %q", balancerModuleMode)
	}

	return nil
}
