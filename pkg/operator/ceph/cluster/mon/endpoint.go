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
func flattenMonEndpoints(mons map[string]*cephclient.MonInfo) string {
	endpoints := []string{}
	for _, m := range mons {
		endpoints = append(endpoints, fmt.Sprintf("%s=%s", m.Name, m.Endpoint))
	}
	return strings.Join(endpoints, ",")
}
