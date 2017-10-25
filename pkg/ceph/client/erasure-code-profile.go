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

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
)

type CephErasureCodeProfile struct {
	DataChunkCount   uint   `json:"k,string"`
	CodingChunkCount uint   `json:"m,string"`
	Plugin           string `json:"plugin"`
	Technique        string `json:"technique"`
	FailureDomain    string `json:"crush-failure-domain"`
}

func ListErasureCodeProfiles(context *clusterd.Context, clusterName string) ([]string, error) {
	args := []string{"osd", "erasure-code-profile", "ls"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to list erasure-code-profiles: %+v", err)
	}

	var ecProfiles []string
	err = json.Unmarshal(buf, &ecProfiles)
	if err != nil {
		return nil, fmt.Errorf("unmarshal failed: %+v. raw buffer response: %s", err, string(buf))
	}

	return ecProfiles, nil
}

func GetErasureCodeProfileDetails(context *clusterd.Context, clusterName, name string) (CephErasureCodeProfile, error) {
	args := []string{"osd", "erasure-code-profile", "get", name}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return CephErasureCodeProfile{}, fmt.Errorf("failed to get erasure-code-profile for '%s': %+v", name, err)
	}

	var ecProfileDetails CephErasureCodeProfile
	err = json.Unmarshal(buf, &ecProfileDetails)
	if err != nil {
		return CephErasureCodeProfile{}, fmt.Errorf("unmarshal failed: %+v. raw buffer response: %s", err, string(buf))
	}

	return ecProfileDetails, nil
}

func CreateErasureCodeProfile(context *clusterd.Context, clusterName string, config model.ErasureCodedPoolConfig, name, failureDomain string) error {
	// look up the default profile so we can use the default plugin/technique
	defaultProfile, err := GetErasureCodeProfileDetails(context, clusterName, "default")
	if err != nil {
		return fmt.Errorf("failed to look up default erasure code profile: %+v", err)
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

	args := []string{"osd", "erasure-code-profile", "set", name}
	args = append(args, profilePairs...)
	_, err = ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to set ec-profile. %+v", err)
	}

	return nil
}

func ModelPoolToCephPool(modelPool model.Pool) CephStoragePoolDetails {
	pool := CephStoragePoolDetails{
		Name:          modelPool.Name,
		Number:        modelPool.Number,
		FailureDomain: modelPool.FailureDomain,
	}

	if modelPool.Type == model.Replicated {
		pool.Size = modelPool.ReplicatedConfig.Size
	} else if modelPool.Type == model.ErasureCoded {
		pool.ErasureCodeProfile = fmt.Sprintf("%s_ecprofile", modelPool.Name)
	}

	return pool
}
