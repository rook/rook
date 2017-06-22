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
package test

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/etcd/client"
	"github.com/coreos/etcd/pkg/types"
	"github.com/rook/rook/pkg/etcd/bootstrap"
	ctx "golang.org/x/net/context"
)

type MockContext struct {
	members    []string
	urlsMap    types.URLsMap
	membersAPI client.MembersAPI
}

func (m *MockContext) Client(initialNodes []string) (client.Client, error) {
	c, err := client.New(client.Config{
		Endpoints:               initialNodes,
		Transport:               client.DefaultTransport,
		HeaderTimeoutPerRequest: 0 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// MembersAPI returns an instance of MembersAPI for the current etcd cluster
func (m *MockContext) MembersAPI() (client.MembersAPI, error) {
	if m.membersAPI == nil {
		m.membersAPI = &MockMembersAPI{}
	}

	return m.membersAPI, nil
}

func (m *MockContext) KeysAPI() (client.KeysAPI, error) {
	members, _, _ := m.Members()
	c, err := m.Client(members)
	if err != nil {
		return nil, err
	}
	kAPI := client.NewKeysAPI(c)
	return kAPI, nil
}

func (m *MockContext) Members() ([]string, types.URLsMap, error) {
	return m.members, m.urlsMap, nil
}

// AddMembers is added for easier unit testing. In real implementation, we always get the latest
// membership from the cluster itself.
func (m *MockContext) AddMembers(members []string) {
	m.members = append(m.members, members...)
	mAPI, _ := m.MembersAPI()
	if m.urlsMap == nil {
		m.urlsMap = types.URLsMap{}
	}
	for _, member := range members {
		uu, _ := url.Parse(member)
		memberURL := "http://" + strings.Split(uu.Host, ":")[0] + ":53380"
		urls, err := types.NewURLs([]string{memberURL})
		if err != nil {
			return // we don't care about the error case as this method is only used for the unit testing.
		}
		mAPI.Add(ctx.Background(), memberURL)
		u, _ := url.Parse(member)
		m.urlsMap[strings.Split(u.Host, ".")[0]] = urls
	}
}

type MockEmbeddedEtcdFactory struct {
}

func (m *MockEmbeddedEtcdFactory) NewEmbeddedEtcd(token string, conf *bootstrap.Config, newCluster bool) (*bootstrap.EmbeddedEtcd, error) {
	return &bootstrap.EmbeddedEtcd{}, nil
}

type MockMembersAPI struct {
	members []client.Member
}

func (m *MockMembersAPI) List(ctx ctx.Context) ([]client.Member, error) {
	return m.members, nil
}

func (m *MockMembersAPI) Add(ctx ctx.Context, peerURL string) (*client.Member, error) {
	hasher := md5.New()
	hasher.Write([]byte(peerURL))
	member := client.Member{PeerURLs: []string{peerURL}, ID: hex.EncodeToString(hasher.Sum(nil))}
	m.members = append(m.members, member)
	return &member, nil
}

func (m *MockMembersAPI) Remove(ctx ctx.Context, mID string) error {
	for i, member := range m.members {
		if member.ID == mID {
			m.members = append(m.members[:i], m.members[i+1:]...)
			return nil
		}
	}
	return errors.New("member not found")
}

func (m *MockMembersAPI) Update(ctx ctx.Context, mID string, peerURLs []string) error {
	return nil
}

func (m *MockMembersAPI) Leader(ctx ctx.Context) (*client.Member, error) {
	return &client.Member{}, nil
}
