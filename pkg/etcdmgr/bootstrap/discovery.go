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
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/coreos/etcd/client"
	"github.com/coreos/etcd/store"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

func isQuorumFull(token string) (bool, error, []string) {
	res, err := queryDiscoveryService(token + "/_config/size")
	if err != nil {
		return false, fmt.Errorf("cannot get discovery url cluster size: %v", err), []string{}
	}

	size, _ := strconv.ParseInt(*res.Node.Value, 10, 16)
	clusterSize := int(size)
	logger.Infof("cluster max size is: %d", clusterSize)

	currentNodes, _ := GetCurrentNodesFromDiscovery(token)
	logger.Infof("currentNodes: %+v", currentNodes)
	if len(currentNodes) < clusterSize {
		return false, nil, []string{}
	}
	return true, nil, currentNodes

}

// queryDiscoveryService reads a key from a discovery url.
func queryDiscoveryService(token string) (*store.Event, error) {
	ctx, _ := context.WithTimeout(context.Background(), DefaultClientTimeout)
	resp, err := ctxhttp.Get(ctx, http.DefaultClient, token)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("status code %d from %q: %s", resp.StatusCode, token, body)
	}

	var res store.Event
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return nil, fmt.Errorf("invalid answer from %q: %v", token, err)
	}

	return &res, nil
}

func GetCurrentNodesFromDiscovery(token string) ([]string, error) {
	res, err := queryDiscoveryService(token)
	if err != nil {
		return nil, err
	}

	nodes := make([]string, 0, len(res.Node.Nodes))
	for _, nn := range res.Node.Nodes {
		if nn.Value == nil {
			logger.Debugf("Skipping %q because no value exists", nn.Key)
		}

		n, err := newDiscoveryNode(*nn.Value, DefaultClientPort)
		if err != nil {
			logger.Warningf("invalid peer url %q in discovery service: %v", *nn.Value, err)
			continue
		}

		for _, node := range n {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
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
