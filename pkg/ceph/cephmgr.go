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
package ceph

import (
	"github.com/rook/rook/pkg/ceph/mds"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/ceph/osd"
	"github.com/rook/rook/pkg/ceph/rgw"
	"github.com/rook/rook/pkg/clusterd"
)

const (
	cephName = "ceph"
)

// create a new ceph service
func NewCephService(devices, metadataDevice, directories string, forceFormat bool,
	location, adminSecret string, storeConfig osd.StoreConfig) *clusterd.ClusterService {

	return &clusterd.ClusterService{
		Name:   cephName,
		Leader: newLeader(adminSecret),
		Agents: []clusterd.ServiceAgent{
			mon.NewAgent(),
			osd.NewAgent(devices, false, metadataDevice, directories, forceFormat, location, storeConfig, nil),
			mds.NewAgent(),
			rgw.NewAgent(),
		},
	}
}
