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
package cephmgr

import (
	"fmt"
	"path"
	"path/filepath"
)

const (
	PrivateIPv4Value       = "privateIPv4"
	monitorKeyringTemplate = `
[mon.]
	key = %s
	caps mon = "allow *"
[client.admin]
	key = %s
	auid = 0
	caps mds = "allow"
	caps mon = "allow *"
	caps osd = "allow *"
`
)

// get the key value store path for a given monitor's endpoint
func getMonitorEndpointKey(name string) string {
	return fmt.Sprintf(path.Join(cephKey, "mons/%s/endpoint"), name)
}

// get the path of a given monitor's run dir
func getMonRunDirPath(configDir, monName string) string {
	return path.Join(configDir, monName)
}

// get the path of a given monitor's keyring
func getMonKeyringPath(configDir, monName string) string {
	return filepath.Join(getMonRunDirPath(configDir, monName), "keyring")
}

// get the path of a given monitor's data dir
func getMonDataDirPath(configDir, monName string) string {
	return filepath.Join(getMonRunDirPath(configDir, monName), fmt.Sprintf("mon.%s", monName))
}
