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
package policy

import (
	"errors"
	"sort"

	"github.com/rook/rook/pkg/util"
)

// ChooseEtcdCandidatesToAddOrRemove decides which node to become member..
func ChooseEtcdCandidatesToAddOrRemove(delta int, currentEtcdNodes, currentNodes, unhealthyNodes []string) ([]string, error) {
	var candidates []string
	if delta > 0 {
		// We assume unhealthynode event happens separate from add node event. So, in this case unhealthyNodes
		// should be empty
		if len(unhealthyNodes) > 0 {
			return []string{}, errors.New("request for adding nodes to the etcd cluster, while there are unhealthy nodes")
		}
		candidates = util.SetDifference(currentNodes, currentEtcdNodes).ToSlice()
		if len(candidates) > delta {
			return candidates[:delta], nil
		}
	} else {
		// Choosing etcd nodes to remove
		currentEtcdNodeSet := util.CreateSet(currentEtcdNodes)
		currentEtcdNodeSet.Subtract(util.SetDifference(currentEtcdNodes, unhealthyNodes))
		unhealthyEtcdNodes := currentEtcdNodeSet.ToSlice()
		logger.Infof("unhealthyEtcdNodes: %+v", unhealthyEtcdNodes)
		candidates = unhealthyEtcdNodes
		numOfNodeToRemove := -delta
		if len(candidates) >= numOfNodeToRemove {
			sort.Strings(candidates)
			return candidates[:numOfNodeToRemove], nil
		}

		//need more candidates from etcd nodes
		numOfNodeToRemove = numOfNodeToRemove - len(candidates)
		moreCandidates := util.SetDifference(currentEtcdNodes, candidates).ToSlice()
		sort.Strings(moreCandidates)
		if len(moreCandidates) >= numOfNodeToRemove {
			candidates = append(candidates, moreCandidates[:numOfNodeToRemove]...)
			sort.Strings(candidates)
			return candidates, nil
		}
		candidates = append(candidates, moreCandidates...)
	}

	sort.Strings(candidates)
	return candidates, nil
}

// CalculateDesiredEtcdCount the number of etcd instances that should be deployed
// TODO: make the logic more intelligent.
func CalculateDesiredEtcdCount(nodeCount int) int {
	if nodeCount > 100 {
		return 7
	} else if nodeCount > 20 {
		return 5
	} else if nodeCount > 2 {
		return 3
	}
	return 1
}
