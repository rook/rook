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

package config

import (
	"fmt"

	rookceph "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
)

// MonStore provides methods for setting Ceph configurations in the centralized mon
// configuration database.
type MonStore struct {
	context   *clusterd.Context
	namespace string
}

// GetMonStore returns a new MonStore for the cluster.
func GetMonStore(context *clusterd.Context, namespace string) *MonStore {
	return &MonStore{
		context:   context,
		namespace: namespace,
	}
}

// Set sets a config in the centralized mon configuration database.
// https://docs.ceph.com/docs/master/rados/configuration/ceph-conf/#monitor-configuration-database
// If the value is a nil pointer, the config is instead removed, allowing the config to take on the
// default value.
func (m *MonStore) Set(who, option string, value *string) error {
	var args []string
	var cephCmd *client.CephToolCommand
	if value != nil {
		args = []string{"config", "set", who, normalizeKey(option), *value}
		cephCmd = client.NewCephCommand(m.context, m.namespace, args)
	} else {
		args = []string{"config", "rm", who, normalizeKey(option)}
		cephCmd = client.NewCephCommand(m.context, m.namespace, args)
	}

	out, err := cephCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to set Ceph config in the centralized mon configuration database; "+
			"you may need to use the rook-config-override ConfigMap. output: %s. %+v", string(out), err)
	}
	return nil
}

// SetAll sets all configs from the overrides in the centralized mon configuration database.
// See MonStore.Set for more.
func (m *MonStore) SetAll(configOverrides rookceph.ConfigOverridesSpec) error {
	var errs []error
	for _, override := range configOverrides {
		err := m.Set(override.Who, override.Option, override.Value)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		retErr := fmt.Errorf("failed to set one or more Ceph configs")
		for _, err := range errs {
			retErr = fmt.Errorf("%+v. %+v", retErr, err)
		}
		return retErr
	}
	return nil
}
