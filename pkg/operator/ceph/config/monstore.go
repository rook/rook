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
	"github.com/pkg/errors"
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

// Option defines the pieces of information relevant to Ceph configuration options.
type Option struct {
	// Who is the entity(-ies) the option should apply to.
	Who string

	// Option is the option key
	Option string

	// Value is the value for the option
	Value string
}

// Set sets a config in the centralized mon configuration database.
// https://docs.ceph.com/docs/master/rados/configuration/ceph-conf/#monitor-configuration-database
func (m *MonStore) Set(who, option, value string) error {
	args := []string{"config", "set", who, normalizeKey(option), value}
	cephCmd := client.NewCephCommand(m.context, m.namespace, args)
	out, err := cephCmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set ceph config in the centralized mon configuration database; "+
			"you may need to use the rook-config-override ConfigMap. output: %s", string(out))
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
