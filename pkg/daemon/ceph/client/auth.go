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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/util/exec"
)

// KeyTypeFlag is the flag used by some `ceph auth` commands to specify the CephX key (cipher) type
const KeyTypeFlag = "--key-type"

// AuthListOutput contains list of ceph user details contains entries, keys.
type AuthListOutput struct {
	AuthDump []AuthListEntry `json:"auth_dump"`
}

// AuthListEntry contains only entity field for each user.
type AuthListEntry struct {
	Entity string `json:"entity"`
}

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
func AuthGetOrCreateKey(context *clusterd.Context, clusterInfo *ClusterInfo, name, keyType string, caps []string) (string, error) {
	logger.Infof("getting or creating ceph auth key %q", name)
	args := append([]string{"auth", "get-or-create-key", name}, caps...)
	// allow specifying keyType='aes' even when Ceph doesn't know about key types
	if !Aes256kKeysSupported(clusterInfo.CephVersion) && IsLegacyKeyType(keyType) {
		logger.Infof("for cluster in namespace %q, not specifying key type for get-or-create-key %q because Ceph version %q does not support key types",
			clusterInfo.Namespace, name, clusterInfo.CephVersion.String())
		keyType = "" // don't set --key-type flag
	}
	if keyType != "" {
		args = append(args, KeyTypeFlag, keyType)
	}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		logger.Tracef("failed get-or-create-key reason: %s", string(buf)) // only trace(insecure) log this because it could contain sensitive key info
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

// IsLegacyKeyType returns true if the key type is "legacy", meaning it predates Ceph's support for
// `--key-type` arguments.
func IsLegacyKeyType(keyType string) bool {
	return keyType == string(cephv1.CephxKeyTypeAes)
}

// Aes256kKeysSupported returns true if the given Ceph version supports aes256k keys
func Aes256kKeysSupported(ver version.CephVersion) bool {
	switch ver.Major {
	case 19:
		return ver.IsAtLeast(version.CephVersion{Major: 19, Minor: 2, Extra: 6})
	case 20:
		return ver.IsAtLeast(version.CephVersion{Major: 20, Minor: 2, Extra: 3})
	default:
		return ver.Major >= 21
	}
}

// AuthRotate rotates a daemon's cephx auth key, retaining existing caps.
func AuthRotate(context *clusterd.Context, clusterInfo *ClusterInfo, name, keyType string) (string, error) {
	logger.Infof("rotating ceph auth key %q", name)
	args := []string{"auth", "rotate", name}
	// allow specifying keyType='aes' even when Ceph doesn't know about key types
	if !Aes256kKeysSupported(clusterInfo.CephVersion) && IsLegacyKeyType(keyType) {
		logger.Infof("for cluster in namespace %q, not specifying key type for auth rotate %q because Ceph version %q does not support key types",
			clusterInfo.Namespace, name, clusterInfo.CephVersion.String())
		keyType = "" // don't set --key-type flag
	}
	if keyType != "" {
		args = append(args, KeyTypeFlag, keyType)
	}
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

// AuthList will list all the ceph user.
func AuthList(context *clusterd.Context, clusterInfo *ClusterInfo) (AuthListOutput, error) {
	authArgs := []string{"auth", "ls"}
	output, err := NewCephCommand(context, clusterInfo, authArgs).Run()
	if err != nil {
		return AuthListOutput{}, errors.Wrap(err, "failed to list ceph auth ls")
	}

	var auth AuthListOutput
	err = json.Unmarshal(output, &auth)
	if err != nil {
		// insecure trace logging will show the raw response if debugging required
		logger.Tracef("failed to unmarshal auth ls response: %s", string(output))
		return auth, errors.Wrap(err, "failed to unmarshal auth ls response")
	}

	return auth, err
}

// KeyDumpSecret represents detailed info about a CephX key.
type KeyDumpSecret struct {
	Entity struct {
		TypeStr string `json:"type_str"`
		Id      string `json:"id"`
	} `json:"entity"`

	Auth struct {
		Key struct {
			TypeStr string `json:"type_str"`
		} `json:"key"`
	} `json:"auth"`
}

// AuthDumpKeysOutput represents the output of `ceph auth dump-keys --format=json`.
type AuthDumpKeysOutput struct {
	Data struct {
		Secrets []KeyDumpSecret `json:"secrets"`
	} `json:"data"`
}

type AuthDumpKeysEntityType string

const (
	AuthDumpKeysEntityTypeMgr    AuthDumpKeysEntityType = "mgr"    // one of the 4 core entity types
	AuthDumpKeysEntityTypeOsd    AuthDumpKeysEntityType = "osd"    // one of the 4 core entity types
	AuthDumpKeysEntityTypeMds    AuthDumpKeysEntityType = "mds"    // one of the 4 core entity types
	AuthDumpKeysEntityTypeClient AuthDumpKeysEntityType = "client" // all other entities are 'client'
)

// AuthDumpKeys dumps all CephX keys and returns detailed info about them.
// This is used to determine the key type associated with each CephX key.
// Note: the mon key is not included in this output.
func AuthDumpKeys(context *clusterd.Context, clusterInfo *ClusterInfo) (AuthDumpKeysOutput, error) {
	authArgs := []string{"auth", "dump-keys"}
	output, err := NewCephCommand(context, clusterInfo, authArgs).Run()
	if err != nil {
		return AuthDumpKeysOutput{}, errors.Wrap(err, "failed to dump ceph auth keys")
	}

	var authKeys AuthDumpKeysOutput
	err = json.Unmarshal(output, &authKeys)
	if err != nil {
		return AuthDumpKeysOutput{}, errors.Wrap(err, "failed to unmarshal ceph auth keys")
	}
	return authKeys, nil
}
