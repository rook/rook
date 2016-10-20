package policy

import (
	"errors"
	"log"
	"sort"

	"github.com/quantum/castle/pkg/util"
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
		log.Println("unhealthyEtcdNodes: ", unhealthyEtcdNodes)
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
