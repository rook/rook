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

	"github.com/rook/rook/pkg/daemon/ceph/client"
)

// IsClusterHealthy determines if the Rook cluster is currently healthy or not.
func IsClusterHealthy(testClient *TestClient, namespace string) (bool, error) {

	status, err := testClient.Status(namespace)
	if err != nil {
		return false, err
	}
	logger.Infof("cluster status: %+v", status)

	// verify all mons are in quorum
	if len(status.Quorum) == 0 {
		return false, fmt.Errorf("too few monitors: %+v", status)
	}
	for _, mon := range status.MonMap.Mons {
		if !monInQuorum(mon, status.Quorum) {
			return false, fmt.Errorf("mon %s not in quorum: %v", mon.Name, status.Quorum)
		}
	}

	// verify there are OSDs and they are all up/in
	totalOSDs := status.OsdMap.OsdMap.NumOsd
	if totalOSDs == 0 {
		return false, fmt.Errorf("no OSDs: %+v", status)
	}
	if status.OsdMap.OsdMap.NumInOsd != totalOSDs || status.OsdMap.OsdMap.NumUpOsd != totalOSDs {
		return false, fmt.Errorf("not all OSDs are up/in: %+v", status)
	}

	// verify MGRs are available
	if !status.MgrMap.Available {
		return false, fmt.Errorf("MGRs are not available: %+v", status)
	}

	// verify that all PGs are in the active+clean state (0 PGs is OK because that means no pools
	// have been created yet)
	if status.PgMap.NumPgs > 0 {
		activeCleanCount := 0
		for _, pg := range status.PgMap.PgsByState {
			if pg.StateName == "active+clean" {
				activeCleanCount = pg.Count
				break
			}
		}
		if activeCleanCount != status.PgMap.NumPgs {
			return false, fmt.Errorf("not all PGs are active+clean: %+v", status.PgMap)
		}
	}

	// cluster passed all the basic health checks, seems healthy
	return true, nil
}

func monInQuorum(mon client.MonMapEntry, quorum []int) bool {
	for _, entry := range quorum {
		if entry == mon.Rank {
			return true
		}
	}
	return false
}
