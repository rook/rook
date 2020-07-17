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

package mon

import (
	"fmt"
	"strings"

	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
)

// FlattenMonEndpoints returns a comma-delimited string of all mons and endpoints in the form
// <mon-name>=<mon-endpoint>
func FlattenMonEndpoints(mons map[string]*cephclient.MonInfo) string {
	endpoints := []string{}
	for _, m := range mons {
		endpoints = append(endpoints, fmt.Sprintf("%s=%s", m.Name, m.Endpoint))
	}
	return strings.Join(endpoints, ",")
}

// ParseMonEndpoints parses a flattened representation of mons and endpoints in the form
// <mon-name>=<mon-endpoint> and returns a list of Ceph mon configs.
func ParseMonEndpoints(input string) map[string]*cephclient.MonInfo {
	logger.Infof("parsing mon endpoints: %s", input)
	mons := map[string]*cephclient.MonInfo{}
	rawMons := strings.Split(input, ",")
	for _, rawMon := range rawMons {
		parts := strings.Split(rawMon, "=")
		if len(parts) != 2 {
			logger.Warningf("ignoring invalid monitor %s", rawMon)
			continue
		}
		mons[parts[0]] = &cephclient.MonInfo{Name: parts[0], Endpoint: parts[1]}
	}
	return mons
}
