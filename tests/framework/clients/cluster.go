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

package clients

import (
	"fmt"

	"github.com/rook/rook/pkg/model"
)

// IsClusterHealthy determines if the Rook cluster is currently healthy or not.
func IsClusterHealthy(testClient *TestClient) (model.StatusDetails, error) {

	status, err := testClient.Status()
	if err != nil {
		return status, err
	}

	// verify all mons are in quorum
	if len(status.Monitors) < 3 {
		return status, fmt.Errorf("too few monitors: %+v", status)
	}
	for _, m := range status.Monitors {
		if !m.InQuorum {
			return status, fmt.Errorf("mon %s not in quorum: %+v", m.Name, status)
		}
	}

	// verify there are OSDs and they are all up/in
	if status.OSDs.Total == 0 {
		return status, fmt.Errorf("no OSDs: %+v", status)
	}
	if status.OSDs.NumberUp != status.OSDs.Total || status.OSDs.NumberIn != status.OSDs.Total {
		return status, fmt.Errorf("not all OSDs are up/in: %+v", status)
	}

	// verify MGRs are available
	if !status.Mgrs.Available {
		return status, fmt.Errorf("MGRs are not available: %+v", status)
	}

	// verify that all PGs are in the active+clean state (0 PGs is OK because that means no pools
	// have been created yet)
	if status.PGs.Total > 0 {
		activeCleanCount, ok := status.PGs.StateCounts["active+clean"]
		if !ok || activeCleanCount != status.PGs.Total {
			return status, fmt.Errorf("not all PGs are active+clean: %+v", status)
		}
	}

	// cluster passed all the basic health checks, seems healthy
	return status, nil
}
