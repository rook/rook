/*
Copyright 2021 The Rook Authors. All rights reserved.
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

// Package controller provides Kubernetes controller/pod/container spec items used for many Ceph daemons
package controller

// PeerToken is the content of the peer token
type PeerToken struct {
	ClusterFSID string `json:"fsid"`
	ClientID    string `json:"client_id"`
	Key         string `json:"key"`
	MonHost     string `json:"mon_host"`
	// These fields are added by Rook and NOT part of the output of client.CreateRBDMirrorBootstrapPeer()
	PoolID    int    `json:"pool_id"`
	Namespace string `json:"namespace"`
}
