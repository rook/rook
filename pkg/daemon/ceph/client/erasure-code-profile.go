/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"fmt"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/model"
)

type CephErasureCodeProfile struct {
	DataChunkCount   uint   `json:"k,string"`
	CodingChunkCount uint   `json:"m,string"`
	Plugin           string `json:"plugin"`
	Technique        string `json:"technique"`
	FailureDomain    string `json:"crush-failure-domain"`
	CrushRoot        string `json:"crush-root"`
}

func ListErasureCodeProfiles(context *clusterd.Context, clusterName string) ([]string, error) {
	args := []string{"osd", "erasure-code-profile", "ls"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list erasure-code-profiles")
	}

	var ecProfiles []string
	err = json.Unmarshal(buf, &ecProfiles)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal failed raw buffer response %s", string(buf))
	}

	return ecProfiles, nil
}

func GetErasureCodeProfileDetails(context *clusterd.Context, clusterName, name string) (CephErasureCodeProfile, error) {
	args := []string{"osd", "erasure-code-profile", "get", name}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return CephErasureCodeProfile{}, errors.Wrapf(err, "failed to get erasure-code-profile for %q", name)
	}

	var ecProfileDetails CephErasureCodeProfile
	err = json.Unmarshal(buf, &ecProfileDetails)
	if err != nil {
		return CephErasureCodeProfile{}, errors.Wrapf(err, "unmarshal failed raw buffer response %s", string(buf))
	}

	return ecProfileDetails, nil
}

func CreateErasureCodeProfile(context *clusterd.Context, clusterName string, config model.ErasureCodedPoolConfig, name, failureDomain, crushRoot, deviceClass string) error {
	// look up the default profile so we can use the default plugin/technique
	defaultProfile, err := GetErasureCodeProfileDetails(context, clusterName, "default")
	if err != nil {
		return errors.Wrapf(err, "failed to look up default erasure code profile")
	}

	// define the profile with a set of key/value pairs
	profilePairs := []string{
		fmt.Sprintf("k=%d", config.DataChunkCount),
		fmt.Sprintf("m=%d", config.CodingChunkCount),
		fmt.Sprintf("plugin=%s", defaultProfile.Plugin),
		fmt.Sprintf("technique=%s", defaultProfile.Technique),
	}
	if failureDomain != "" {
		profilePairs = append(profilePairs, fmt.Sprintf("crush-failure-domain=%s", failureDomain))
	}
	if crushRoot != "" {
		profilePairs = append(profilePairs, fmt.Sprintf("crush-root=%s", crushRoot))
	}
	if deviceClass != "" {
		profilePairs = append(profilePairs, fmt.Sprintf("crush-device-class=%s", deviceClass))
	}

	args := []string{"osd", "erasure-code-profile", "set", name}
	args = append(args, profilePairs...)
	_, err = NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set ec-profile")
	}

	return nil
}

func DeleteErasureCodeProfile(context *clusterd.Context, clusterName string, erasureCodeProfile string) error {
	args := []string{"osd", "erasure-code-profile", "rm", erasureCodeProfile}

	cmd := NewCephCommand(context, clusterName, args)
	cmd.JsonOutput = false
	buf, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to delete erasure-code-profile %q. output: %q.", erasureCodeProfile, string(buf))
	}

	return nil
}

func ModelPoolToCephPool(modelPool model.Pool) CephStoragePoolDetails {
	pool := CephStoragePoolDetails{
		Name:             modelPool.Name,
		Number:           modelPool.Number,
		FailureDomain:    modelPool.FailureDomain,
		CrushRoot:        modelPool.CrushRoot,
		DeviceClass:      modelPool.DeviceClass,
		NotEnableAppPool: modelPool.NotEnableAppPool,
	}

	if modelPool.Type == model.Replicated {
		pool.Size = modelPool.ReplicatedConfig.Size
	} else if modelPool.Type == model.ErasureCoded {
		pool.ErasureCodeProfile = GetErasureCodeProfileForPool(modelPool.Name)
	}

	return pool
}

func GetErasureCodeProfileForPool(poolName string) string {
	return fmt.Sprintf("%s_ecprofile", poolName)
}
