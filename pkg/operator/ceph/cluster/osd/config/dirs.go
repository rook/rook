/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package config for OSD config managed by the operator
package config

import (
	"encoding/json"

	"github.com/rook/rook/pkg/operator/k8sutil"
)

func LoadOSDDirMap(kv *k8sutil.ConfigMapKVStore, nodeName string) (map[string]int, error) {
	dirMapRaw, err := kv.GetValue(GetConfigStoreName(nodeName), osdDirsKeyName)
	if err != nil {
		return nil, err
	}

	var dirMap map[string]int
	err = json.Unmarshal([]byte(dirMapRaw), &dirMap)
	if err != nil {
		return nil, err
	}

	return dirMap, nil
}

func SaveOSDDirMap(kv *k8sutil.ConfigMapKVStore, nodeName string, dirMap map[string]int) error {
	if len(dirMap) == 0 {
		return nil
	}

	b, err := json.Marshal(dirMap)
	if err != nil {
		return err
	}

	err = kv.SetValue(GetConfigStoreName(nodeName), osdDirsKeyName, string(b))
	if err != nil {
		return err
	}

	return nil
}
