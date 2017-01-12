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
package cephmgr

import (
	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/mds"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/cephmgr/osd"
	"github.com/rook/rook/pkg/cephmgr/osd/partition"
	"github.com/rook/rook/pkg/cephmgr/rgw"
	"github.com/rook/rook/pkg/clusterd"
)

const (
	cephName = "ceph"
)

// create a new ceph service
func NewCephService(factory client.ConnectionFactory, devices, metadataDevice string, forceFormat bool,
	location, adminSecret string, bluestoreConfig partition.BluestoreConfig) *clusterd.ClusterService {

	return &clusterd.ClusterService{
		Name:   cephName,
		Leader: newLeader(factory, adminSecret),
		Agents: []clusterd.ServiceAgent{
			mon.NewAgent(factory),
			osd.NewAgent(factory, devices, metadataDevice, forceFormat, location, bluestoreConfig),
			mds.NewAgent(factory),
			rgw.NewAgent(factory),
		},
	}
}
