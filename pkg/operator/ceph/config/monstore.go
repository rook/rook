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
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/exec"
	"golang.org/x/exp/slices"
	"gopkg.in/ini.v1"
)

// Alias for cluster CRD ceph config options map
type CephConfigOptionsMap = map[string]map[string]string

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

// SetIfChanged sets a config in the centralized mon configuration database if the config has
// changed value.
// https://docs.ceph.com/docs/master/rados/configuration/ceph-conf/#monitor-configuration-database
//
// There is a bug through at least Ceph v18 where `ceph config get global <option>` does not work.
// As a workaround it is possible to use `ceph config get client <option>` as long as the config
// option won't be overridden by clients. SetIfChanged uses this workaround assuming it is valid.
// Any new uses of this function should take extreme care when using `who="global"` to check that
// the workaround is valid for usage with the given option.
// Options validated for workaround by Ceph devs: public_network, cluster_network
func (m *MonStore) SetIfChanged(who, option, value string) (bool, error) {
	getWho := who
	if who == "global" {
		getWho = "client"
	}
	currentVal, err := m.Get(getWho, option)
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
	logger.Infof("setting option %q (user %q) to the mon configuration database", option, who)
	args := []string{"config", "set", who, normalizeKey(option), value}
	cephCmd := client.NewCephCommand(m.context, m.clusterInfo, args)
	out, err := cephCmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to set ceph config in the centralized mon configuration database; "+
			"you may need to use the rook-config-override ConfigMap. output: %s", string(out))
	}

	logger.Infof("successfully set option %q (user %q) to the mon configuration database", option, who)
	return nil
}

// Delete a config in the centralized mon configuration database.
func (m *MonStore) Delete(who, option string) error {
	logger.Infof("deleting %q %q option from the mon configuration database", who, option)
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

// SetKeyValue sets an arbitrary key/value pair in Ceph's general purpose (as opposed to
// configuration-specific) key/value store. Keys and values can be any arbitrary string including
// spaces, underscores, dashes, and slashes.
// See: https://docs.ceph.com/en/latest/man/8/ceph/#config-key
func (m *MonStore) SetKeyValue(key, value string) error {
	logger.Debugf("setting %q option in the mon config-key store", key)
	args := []string{"config-key", "set", key, value}
	cephCmd := client.NewCephCommand(m.context, m.clusterInfo, args)
	out, err := cephCmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to set %q in the mon config-key store. output: %s", key, string(out))
	}
	return nil
}

func (m *MonStore) SetAllMultiple(settings map[string]map[string]string) error {
	for who, options := range settings {
		if err := m.SetAll(who, options); err != nil {
			return errors.Wrapf(err, "failed to set ceph config for target: %s", who)
		}
	}

	return nil
}

func (m *MonStore) SetAll(who string, settings map[string]string) error {
	keys, err := m.setAll(who, settings)
	if err != nil {
		return errors.Wrapf(err, "failed to set all keys")
	}
	if len(keys) == 0 {
		return nil
	}
	logger.Infof("failed to set keys %v, trying to remove them first", keys)
	newSettings := map[string]string{}
	for _, key := range keys {
		if err := m.Delete(who, key); err != nil {
			return errors.Wrapf(err, "failed to remove key %q", key)
		}
		newSettings[key] = settings[key]
	}
	// retry setting the removed keys
	keys, err = m.setAll(who, newSettings)
	if err != nil {
		return errors.Wrapf(err, "failed to set keys")
	}
	if len(keys) != 0 {
		return errors.Errorf("failed to set keys %v", keys)
	}

	return nil
}

func (m *MonStore) setAll(who string, settings map[string]string) ([]string, error) {
	assimilateConfPath, err := os.CreateTemp(m.context.ConfigDir, "")
	if err != nil {
		return []string{}, errors.Wrapf(err, "failed to create assimilateConf temp dir for  %s.", who)
	}

	err = os.WriteFile(assimilateConfPath.Name(), []byte(""), 0600)
	if err != nil {
		rook.TerminateFatal(errors.Wrapf(err, "failed to write config file"))
	}

	outFilePath := assimilateConfPath.Name() + ".out"
	defer func() {
		err := os.Remove(assimilateConfPath.Name())
		if err != nil {
			logger.Errorf("failed to remove file %q. %v", assimilateConfPath.Name(), err)
		}
		err = os.Remove(outFilePath)
		if err != nil {
			logger.Errorf("failed to remove file %q. %v", outFilePath, err)
		}
	}()

	configFile := ini.Empty()
	s, err := configFile.NewSection(who)
	if err != nil {
		return []string{}, err
	}

	for key, val := range settings {
		if _, err := s.NewKey(key, val); err != nil {
			return []string{}, errors.Wrapf(err, "failed to add key %s", key)
		}
	}

	if err := configFile.SaveTo(assimilateConfPath.Name()); err != nil {
		return []string{}, errors.Wrapf(err, "failed to save config file %s", assimilateConfPath.Name())
	}

	fileContent, err := os.ReadFile(assimilateConfPath.Name())
	if err != nil {
		logger.Errorf("failed to open assimilate input file %s. %c", assimilateConfPath.Name(), err)
	}
	logger.Infof("applying ceph settings:\n%s", string(fileContent))

	args := []string{"config", "assimilate-conf", "-i", assimilateConfPath.Name(), "-o", outFilePath}
	cephCmd := client.NewCephCommand(m.context, m.clusterInfo, args)

	out, err := cephCmd.RunWithTimeout(exec.CephCommandsTimeout)
	fileContent, readErr := os.ReadFile(outFilePath)
	if readErr != nil {
		logger.Errorf("failed to open assimilate output file %s. %v", outFilePath, readErr)
	}
	if err != nil {
		logger.Errorf("failed to run command ceph %s", args)
		logger.Errorf("failed to apply ceph settings:\n%s", string(fileContent))

		return []string{}, errors.Wrapf(err, "failed to set ceph config in the centralized mon configuration database; "+
			"output: %s", string(out))
	}
	if len(fileContent) > 0 {
		logger.Infof("output: %s\n", string(fileContent))
		// read fileContent to ini format
		iniContent, err := ini.Load(fileContent)
		if err != nil {
			return []string{}, errors.Wrapf(err, "failed to parse assimilate output file %s", outFilePath)
		}
		// get the section for the client
		section, err := iniContent.GetSection(who)
		if err != nil {
			return []string{}, errors.Wrapf(err, "failed to get section %s", who)
		}
		return section.KeyStrings(), nil
	}
	logger.Info("successfully applied settings to the mon configuration database")
	return []string{}, nil
}

var criticalConfigOptions = []string{
	"mon_host",
	"fsid",
	"keyring",
}

func (m *MonStore) UpdateConfigStoreFromMap(cfg CephConfigOptionsMap) error {
	filtered := filterSettingsMap(cfg)

	return m.SetAllMultiple(filtered)
}

// Filters out critical config options
func filterSettingsMap(cfg CephConfigOptionsMap) CephConfigOptionsMap {
	filtered := CephConfigOptionsMap{}

	for who, options := range cfg {
		filtered[who] = map[string]string{}
		for k, v := range options {
			if slices.Contains(criticalConfigOptions, k) {
				continue
			}

			filtered[who][k] = v
		}
	}

	return filtered
}
