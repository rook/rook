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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
)

type CephErasureCodeProfile struct {
	DataChunkCount   uint   `json:"k,string"`
	CodingChunkCount uint   `json:"m,string"`
	Plugin           string `json:"plugin"`
	Technique        string `json:"technique"`
	FailureDomain    string `json:"crush-failure-domain"`
	CrushRoot        string `json:"crush-root"`
}

func ListErasureCodeProfiles(context *clusterd.Context, clusterInfo *ClusterInfo) ([]string, error) {
	args := []string{"osd", "erasure-code-profile", "ls"}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list erasure-code-profiles")
	}

	var ecProfiles []string
	err = json.Unmarshal(buf, &ecProfiles)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal failed raw buffer response %s", string(buf))
	}

	return ecProfiles, nil
}

func GetErasureCodeProfileDetails(context *clusterd.Context, clusterInfo *ClusterInfo, name string) (CephErasureCodeProfile, error) {
	args := []string{"osd", "erasure-code-profile", "get", name}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
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

func CreateErasureCodeProfile(context *clusterd.Context, clusterInfo *ClusterInfo, profileName string, pool cephv1.PoolSpec) error {
	// look up the default profile so we can use the default plugin/technique
	defaultProfile, err := GetErasureCodeProfileDetails(context, clusterInfo, "default")
	if err != nil {
		return errors.Wrap(err, "failed to look up default erasure code profile")
	}

	if pool.ErasureCoded.Algorithm != "" {
		defaultProfile.Plugin = pool.ErasureCoded.Algorithm
	}

	// define the profile with a set of key/value pairs
	profilePairs := []string{
		fmt.Sprintf("k=%d", pool.ErasureCoded.DataChunks),
		fmt.Sprintf("m=%d", pool.ErasureCoded.CodingChunks),
		fmt.Sprintf("plugin=%s", defaultProfile.Plugin),
		fmt.Sprintf("technique=%s", defaultProfile.Technique),
	}
	if pool.FailureDomain != "" {
		profilePairs = append(profilePairs, fmt.Sprintf("crush-failure-domain=%s", pool.FailureDomain))
	}
	if pool.CrushRoot != "" {
		profilePairs = append(profilePairs, fmt.Sprintf("crush-root=%s", pool.CrushRoot))
	}
	if pool.DeviceClass != "" {
		profilePairs = append(profilePairs, fmt.Sprintf("crush-device-class=%s", pool.DeviceClass))
	}

	args := []string{"osd", "erasure-code-profile", "set", profileName, "--force"}
	args = append(args, profilePairs...)
	_, err = NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrap(err, "failed to set ec-profile")
	}

	return nil
}

func DeleteErasureCodeProfile(context *clusterd.Context, clusterInfo *ClusterInfo, profileName string) error {
	args := []string{"osd", "erasure-code-profile", "rm", profileName}

	cmd := NewCephCommand(context, clusterInfo, args)
	cmd.JsonOutput = false
	buf, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to delete erasure-code-profile %q. output: %q.", profileName, string(buf))
	}

	return nil
}
