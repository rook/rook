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
package util

import (
	"io/ioutil"
	"strings"
)

const maxMachineIDLength = 12

func GetMachineID() (string, error) {
	buf, err := ioutil.ReadFile("/etc/machine-id")
	if err != nil {
		return "", err
	}

	return trimMachineID(string(buf)), nil
}

func trimMachineID(id string) string {
	// Trim the machine ID to a length that is statistically unlikely to collide with another node in the cluster
	// while allowing us to use an ID that is both unique and succinct.
	// Using the birthday collision algorithm, if we have a length of 12 hex characters, that gives us
	// 16^12 possibilities. If we have a cluster with 1,000 nodes, we have a likelihood with node IDs
	// colliding in less than 1 in a billion clusters.
	id = strings.TrimSpace(id)
	if len(id) <= maxMachineIDLength {
		return id
	}

	return id[0:maxMachineIDLength]
}
