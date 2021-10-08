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
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/exec"
)

// MonStore provides methods for setting Ceph configurations in the centralized mon
// configuration database.
type MonStore struct {
	context     *clusterd.Context
	clusterInfo *client.ClusterInfo
}

// GetMonStore returns a new MonStore for the cluster.
func GetMonStore(context *clusterd.Context, clusterInfo *client.ClusterInfo) *MonStore {
	return &MonStore{
		context:     context,
		clusterInfo: clusterInfo,
	}
}

// Option defines the pieces of information relevant to Ceph configuration options.
type Option struct {
	// Who is the entity(-ies) the option should apply to.
	Who string

	// Option is the option key
	Option string

	// Value is the value for the option
	Value string
}

func (m *MonStore) SetIfChanged(who, option, value string) (bool, error) {
	currentVal, err := m.Get(who, option)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get value %q", option)
	}
	if currentVal == value {
		// no need to update the setting
		return false, nil
	}

	if err := m.Set(who, option, value); err != nil {
		return false, errors.Wrapf(err, "failed to set value %s=%s", option, value)
	}
	return true, nil
}

// Set sets a config in the centralized mon configuration database.
// https://docs.ceph.com/docs/master/rados/configuration/ceph-conf/#monitor-configuration-database
func (m *MonStore) Set(who, option, value string) error {
	logger.Infof("setting %q=%q=%q option to the mon configuration database", who, option, value)
	args := []string{"config", "set", who, normalizeKey(option), value}
	cephCmd := client.NewCephCommand(m.context, m.clusterInfo, args)
	out, err := cephCmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to set ceph config in the centralized mon configuration database; "+
			"you may need to use the rook-config-override ConfigMap. output: %s", string(out))
	}

	logger.Infof("successfully set %q=%q=%q option to the mon configuration database", who, option, value)
	return nil
}

// Delete a config in the centralized mon configuration database.
func (m *MonStore) Delete(who, option string) error {
	logger.Infof("deleting %q option from the mon configuration database", option)
	args := []string{"config", "rm", who, normalizeKey(option)}
	cephCmd := client.NewCephCommand(m.context, m.clusterInfo, args)
	out, err := cephCmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to delete ceph config in the centralized mon configuration database. output: %s",
			string(out))
	}

	logger.Infof("successfully deleted %q option from the mon configuration database", option)
	return nil
}

// Get retrieves a config in the centralized mon configuration database.
// https://docs.ceph.com/docs/master/rados/configuration/ceph-conf/#monitor-configuration-database
func (m *MonStore) Get(who, option string) (string, error) {
	args := []string{"config", "get", who, normalizeKey(option)}
	cephCmd := client.NewCephCommand(m.context, m.clusterInfo, args)
	out, err := cephCmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get config setting %q for user %q", option, who)
	}
	return strings.TrimSpace(string(out)), nil
}

// GetDaemon retrieves all configs for a specific daemon in the centralized mon configuration database.
func (m *MonStore) GetDaemon(who string) ([]Option, error) {
	args := []string{"config", "get", who}
	cephCmd := client.NewCephCommand(m.context, m.clusterInfo, args)
	out, err := cephCmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return []Option{}, errors.Wrapf(err, "failed to get config for daemon %q. output: %s", who, string(out))
	}
	var result map[string]interface{}
	err = json.Unmarshal(out, &result)
	if err != nil {
		return []Option{}, errors.Wrapf(err, "failed to parse json config for daemon %q. json: %s", who, string(out))
	}
	daemonOptions := []Option{}
	for k := range result {
		v := result[k].(map[string]interface{})
		optionWho := v["section"].(string)
		// Only get specialized options (don't take global one)
		if optionWho == who {
			daemonOptions = append(daemonOptions, Option{optionWho, k, v["value"].(string)})
		}
	}
	return daemonOptions, nil
}

// DeleteDaemon delete all configs for a specific daemon in the centralized mon configuration database.
func (m *MonStore) DeleteDaemon(who string) error {
	configOptions, err := m.GetDaemon(who)
	if err != nil {
		return errors.Wrapf(err, "failed to get daemon config for %q", who)
	}

	for _, option := range configOptions {
		err := m.Delete(who, option.Option)
		if err != nil {
			return errors.Wrapf(err, "failed to delete option %q on %q", option.Option, who)
		}
	}
	return nil
}

// SetAll sets all configs from the overrides in the centralized mon configuration database.
// See MonStore.Set for more.
func (m *MonStore) SetAll(options ...Option) error {
	var errs []error
	for _, override := range options {
		err := m.Set(override.Who, override.Option, override.Value)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		retErr := errors.New("failed to set one or more Ceph configs")
		for _, err := range errs {
			retErr = errors.Wrapf(err, "%v", retErr)
		}
		return retErr
	}
	return nil
}

// DeleteAll deletes all provided configs from the overrides in the centralized mon configuration database.
// See MonStore.Delete for more.
func (m *MonStore) DeleteAll(options ...Option) error {
	var errs []error
	for _, override := range options {
		err := m.Delete(override.Who, override.Option)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		retErr := errors.New("failed to delete one or more Ceph configs")
		for _, err := range errs {
			retErr = errors.Wrapf(err, "%v", retErr)
		}
		return retErr
	}
	return nil
}
