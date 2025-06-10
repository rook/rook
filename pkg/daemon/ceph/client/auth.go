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
	"syscall"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
)

// AuthGetOrCreate will either get or create a user with the given capabilities.  The keyring for the
// user will be written to the given keyring path.
func AuthGetOrCreate(context *clusterd.Context, clusterInfo *ClusterInfo, name, keyringPath string, caps []string) error {
	logger.Infof("getting or creating ceph auth %q", name)
	args := append([]string{"auth", "get-or-create", name, "-o", keyringPath}, caps...)
	cmd := NewCephCommand(context, clusterInfo, args)
	cmd.JsonOutput = false
	_, err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to auth get-or-create for %s", name)
	}

	return nil
}

// AuthGetKey gets the key for the given user.
func AuthGetKey(context *clusterd.Context, clusterInfo *ClusterInfo, name string) (string, error) {
	logger.Infof("getting ceph auth key %q", name)
	args := []string{"auth", "get-key", name}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return "", errors.Wrapf(err, "failed to get key for %s", name)
	}

	return parseAuthKey(buf)
}

// AuthGetOrCreateKey gets or creates the key for the given user.
func AuthGetOrCreateKey(context *clusterd.Context, clusterInfo *ClusterInfo, name string, caps []string) (string, error) {
	logger.Infof("getting or creating ceph auth key %q", name)
	args := append([]string{"auth", "get-or-create-key", name}, caps...)
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return "", errors.Wrapf(err, "failed get-or-create-key %s", name)
	}

	return parseAuthKey(buf)
}

// AuthUpdateCaps updates the capabilities for the given user.
func AuthUpdateCaps(context *clusterd.Context, clusterInfo *ClusterInfo, name string, caps []string) error {
	logger.Infof("updating ceph auth caps %q to %v", name, caps)
	args := append([]string{"auth", "caps", name}, caps...)
	_, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to update caps for %s", name)
	}
	return err
}

// AuthGetCaps gets the capabilities for the given user.
func AuthGetCaps(context *clusterd.Context, clusterInfo *ClusterInfo, name string) (caps map[string]string, error error) {
	logger.Infof("getting ceph auth caps for %q", name)
	args := []string{"auth", "get", name}
	output, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get caps for %q", name)
	}

	var data []map[string]interface{}
	err = json.Unmarshal(output, &data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal auth get response")
	}
	caps = make(map[string]string)

	if data[0]["caps"].(map[string]interface{})["mon"] != nil {
		caps["mon"] = data[0]["caps"].(map[string]interface{})["mon"].(string)
	}
	if data[0]["caps"].(map[string]interface{})["mds"] != nil {
		caps["mds"] = data[0]["caps"].(map[string]interface{})["mds"].(string)
	}
	if data[0]["caps"].(map[string]interface{})["mgr"] != nil {
		caps["mgr"] = data[0]["caps"].(map[string]interface{})["mgr"].(string)
	}
	if data[0]["caps"].(map[string]interface{})["osd"] != nil {
		caps["osd"] = data[0]["caps"].(map[string]interface{})["osd"].(string)
	}

	return caps, err
}

// AuthRotate rotates a daemon's cephx auth key, retaining existing caps.
func AuthRotate(context *clusterd.Context, clusterInfo *ClusterInfo, name string) (string, error) {
	logger.Infof("rotating ceph auth key %q", name)
	args := []string{"auth", "rotate", name}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.EINVAL) {
			// `ceph auth rotate` is not yet present in all ceph versions. as long as the command
			// invocation is correct, EINVAL means the ceph version doesn't have the rotate
			// subcommand added in: https://github.com/ceph/ceph/pull/58121
			// all version of ceph v20 (tentacle) and higher should have the command present
			return "", errors.Wrapf(err, "failed auth rotate %s. operator or cluster ceph version does not support ceph auth rotate", name)
		}
		return "", errors.Wrapf(err, "failed auth rotate %s", name)
	}

	var data []map[string]interface{}
	err = json.Unmarshal(buf, &data)
	if err != nil {
		return "", errors.Wrapf(err, "failed to unmarshal auth rotate %s response", name)
	}
	if len(data) < 1 {
		return "", errors.Errorf("auth rotate %s returned no results", name)
	}
	if len(data) > 1 {
		logger.Infof("auth rotate %s returned more than 1 result; continuing using the first result", name)
	}

	return data[0]["key"].(string), nil
}

// AuthDelete will delete the given user.
func AuthDelete(context *clusterd.Context, clusterInfo *ClusterInfo, name string) error {
	logger.Infof("deleting ceph auth %q", name)
	args := []string{"auth", "del", name}
	_, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to delete auth for %s", name)
	}
	return nil
}

func parseAuthKey(buf []byte) (string, error) {
	var resp map[string]interface{}
	if err := json.Unmarshal(buf, &resp); err != nil {
		return "", errors.Wrap(err, "failed to unmarshal get/create key response")
	}
	return resp["key"].(string), nil
}
