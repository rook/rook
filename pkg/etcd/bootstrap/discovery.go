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
package bootstrap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/etcd/client"
	"github.com/coreos/etcd/store"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const (
	discoveryRetryAttempts = 10
)

func isQuorumFull(token string) (bool, []string, error) {
	size, err := getClusterSize(token)
	if err != nil {
		return false, nil, err
	}
	logger.Infof("cluster size is: %d", size)

	currentNodes, err := GetCurrentNodesFromDiscovery(token, size)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get etcd members from discovery. %+v", err)
	}
	logger.Infof("currentNodes: %+v", currentNodes)
	if len(currentNodes) < size {
		return false, []string{}, nil
	}
	return true, currentNodes, nil

}

func getClusterSize(token string) (int, error) {
	res, err := queryDiscoveryService(token + "/_config/size")
	if err != nil {
		return -1, fmt.Errorf("cannot get discovery url cluster size: %v", err)
	}

	size, err := strconv.ParseInt(*res.Node.Value, 10, 16)
	if err != nil {
		return -1, fmt.Errorf("failed to read cluster size. %+v", err)
	}
	return int(size), nil
}

// queryDiscoveryService reads a key from a discovery url.
func queryDiscoveryService(token string) (*store.Event, error) {
	var resp *http.Response
	var err error

	defer safeCloseHTTPResponse(resp)

	// retry the http request in case of network errors
	for i := 1; i <= discoveryRetryAttempts; i++ {
		ctx, _ := context.WithTimeout(context.Background(), DefaultClientTimeout)
		resp, err = ctxhttp.Get(ctx, http.DefaultClient, token)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				break
			}
		}

		logger.Warningf("failed to query discovery service on attempt %d/%d. resp=%+v, err=%+v", i, discoveryRetryAttempts, resp, err)
		safeCloseHTTPResponse(resp)

		if i < discoveryRetryAttempts {
			// delay an extra half second for each retry
			delay := time.Duration(i) * 500 * time.Millisecond
			<-time.After(delay)
		}
	}

	if resp != nil {
		var res store.Event
		err = json.NewDecoder(resp.Body).Decode(&res)
		if err != nil {
			return nil, fmt.Errorf("invalid answer from %q: %v", token, err)
		}

		return &res, nil
	}

	return nil, fmt.Errorf("never received response from %q: %+v", token, err)
}

// Get the nodes that have registered with the discovery service.
// We only want to return the number of nodes that are expected for the etcd cluster size.
// Etcd will not allow more etcd servers to start unless the AddMember api is called.
// Here we will ignore any nodes that have registered beyond the expected size.
// It is important to return the nodes with the lowest index for consistent behavior in the cluster.
func GetCurrentNodesFromDiscovery(token string, size int) ([]string, error) {
	res, err := queryDiscoveryService(token)
	if err != nil {
		return nil, err
	}

	var upperIndex uint64
	var ignored []string
	nodeMap := map[uint64][]string{}
	for _, nn := range res.Node.Nodes {
		if nn.Value == nil {
			logger.Debugf("Skipping %q because no value exists", nn.Key)
		}

		endpoints, err := newDiscoveryNode(*nn.Value, DefaultClientPort)
		if err != nil {
			logger.Warningf("invalid peer url %q in discovery service: %v", *nn.Value, err)
			continue
		}

		upperIndex, ignored = addNodeToMap(nodeMap, endpoints, size, nn.CreatedIndex, upperIndex, ignored)
	}

	if len(ignored) > 0 {
		logger.Infof("Ignored extra etcd members: %+v", ignored)
	}

	// create a flat slice from all the nodes' endpoints
	var nodes []string
	for _, endpoints := range nodeMap {
		for _, node := range endpoints {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

func addNodeToMap(nodeMap map[uint64][]string, endpoints []string, size int, index, upperIndex uint64, ignored []string) (uint64, []string) {
	if len(nodeMap) < size {
		// add the node to the expected list since we haven't reached the expected size yet
		nodeMap[index] = endpoints
		if upperIndex < index {
			upperIndex = index
		}
	} else {
		// if the created index is lower than this index, replace the higher index with the lower
		if upperIndex > index {
			ignored = append(ignored, nodeMap[upperIndex]...)
			delete(nodeMap, upperIndex)
			nodeMap[index] = endpoints

			// find the highest index
			upperIndex = 0
			for i := range nodeMap {
				if upperIndex < i {
					upperIndex = i
				}
			}
		} else {
			// ignore nodes that registered after the quorum was full and the index is over the max
			ignored = append(ignored, endpoints...)
		}
	}

	return upperIndex, ignored
}

type Machine struct {
	client.Member
}

func newDiscoveryNode(namedPeerURLs string, clientPort int) ([]string, error) {
	urls := strings.Split(namedPeerURLs, ",")
	n := Machine{
		Member: client.Member{
			PeerURLs:   make([]string, 0, len(urls)),
			ClientURLs: make([]string, 0, len(urls)),
		},
	}
	for _, namedPeerURL := range urls {
		eqc := strings.SplitN(namedPeerURL, "=", 2)
		if n.Name != "" && n.Name != eqc[0] {
			return nil, fmt.Errorf("different names in %s", namedPeerURLs)
		}
		n.Name = eqc[0]
		colc := strings.SplitN(eqc[1], ":", 3)
		n.PeerURLs = append(n.PeerURLs, eqc[1])
		n.ClientURLs = append(n.ClientURLs, fmt.Sprintf("%s:%s:%d", colc[0], colc[1], clientPort))
	}

	return n.ClientURLs, nil
}

// closes the http response body safely, by checking for nil.  Can be called more than once.
func safeCloseHTTPResponse(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
}
