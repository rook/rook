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
	"github.com/coreos/etcd/client"
	"github.com/coreos/etcd/pkg/types"
	ctx "golang.org/x/net/context"
)

type EtcdMgrContext interface {
	Client(initialNodes []string) (client.Client, error)
	MembersAPI() (client.MembersAPI, error)
	KeysAPI() (client.KeysAPI, error)
	Members() ([]string, types.URLsMap, error)
}

type Context struct {
	ClusterToken string
}

func (e *Context) Client(initialNodes []string) (client.Client, error) {
	c, err := client.New(client.Config{
		Endpoints:               initialNodes,
		Transport:               client.DefaultTransport,
		HeaderTimeoutPerRequest: DefaultClientTimeout,
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// MembersAPI returns an instance of MembersAPI for the current etcd cluster
func (e *Context) MembersAPI() (client.MembersAPI, error) {
	members, _, err := e.Members()
	if err != nil {
		return nil, err
	}
	c, err := e.Client(members)
	if err != nil {
		return nil, err
	}
	mAPI := client.NewMembersAPI(c)
	return mAPI, nil
}

func (e *Context) KeysAPI() (client.KeysAPI, error) {
	members, _, err := e.Members()
	if err != nil {
		return nil, err
	}
	c, err := e.Client(members)
	if err != nil {
		return nil, err
	}
	kAPI := client.NewKeysAPI(c)
	return kAPI, nil
}

func (e *Context) Members() ([]string, types.URLsMap, error) {
	urlsMap := types.URLsMap{}
	var nodes []string

	size, err := getClusterSize(e.ClusterToken)
	if err != nil {
		return nodes, urlsMap, err
	}

	initialNodes, err := GetCurrentNodesFromDiscovery(e.ClusterToken, size)
	if err != nil {
		logger.Errorf("error in GetCurrentNodesFromDiscovery: %+v", err)
		return nodes, urlsMap, err
	}

	c, err := e.Client(initialNodes)
	if err != nil {
		return nodes, urlsMap, err
	}

	mAPI := client.NewMembersAPI(c)
	members, err := mAPI.List(ctx.Background())
	if err != nil {
		return nodes, urlsMap, err
	}

	for _, member := range members {
		urls, err := types.NewURLs(member.PeerURLs)
		if err != nil {
			return nodes, urlsMap, err
		}
		urlsMap[member.ID] = urls
		// ClientURLs of a member is a url which is used by this member to listen to the clients' requests. This url could be used
		// to create etcd client objects. In some use cases, multiple urls might be used, but we don't use that pattern.
		nodes = append(nodes, member.ClientURLs...)
	}
	return nodes, urlsMap, nil
}
