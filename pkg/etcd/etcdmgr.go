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
package etcd

import (
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/etcd/bootstrap"
)

const (
	etcdMgrName = "etcdmgr"
)

// NewEtcdMgrService creates a new etcdmgr service
func NewEtcdMgrService(token string) *clusterd.ClusterService {
	logger.Debugf("creating instances of etcdMgrLeader and etcdMgrAgent")

	return &clusterd.ClusterService{
		Name:   etcdMgrName,
		Leader: &etcdMgrLeader{context: &bootstrap.Context{ClusterToken: token}},
		Agents: []clusterd.ServiceAgent{
			&etcdMgrAgent{context: &bootstrap.Context{ClusterToken: token}, etcdFactory: &bootstrap.EmbeddedEtcdFactory{}},
		},
	}
}
