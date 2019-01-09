/*
Copyright 2017 The Rook Authors. All rights reserved.

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

package mon

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

const (
	maxPerChar = 26
)

func monInQuorum(monitor client.MonMapEntry, quorum []int) bool {
	for _, rank := range quorum {
		if rank == monitor.Rank {
			return true
		}
	}
	return false
}

// convert the mon name to the numeric mon ID
func fullNameToIndex(name string) (int, error) {
	prefix := appName + "-"
	if strings.Index(name, prefix) != -1 && len(prefix) < len(name) {
		return k8sutil.NameToIndex(name[len(prefix)+1:])
	}

	// attempt to parse the legacy mon name
	legacyPrefix := appName
	if strings.Index(name, legacyPrefix) == -1 || len(name) < len(appName) {
		return -1, fmt.Errorf("unexpected mon name")
	}
	id, err := strconv.Atoi(name[len(legacyPrefix):])
	if err != nil {
		return -1, err
	}
	return id, nil
}

// getPortFromEndpoint return the port from an endpoint string (192.168.0.1:6790)
func getPortFromEndpoint(endpoint string) int32 {
	port := DefaultPort
	_, portString, err := net.SplitHostPort(endpoint)
	if err != nil {
		logger.Errorf("failed to split host and port for endpoint %s, assuming default Ceph port %d", endpoint, port)
	} else {
		port, _ = strconv.Atoi(portString)
	}
	return int32(port)
}
