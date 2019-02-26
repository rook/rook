/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package provisioner

import (
	"strings"

	"k8s.io/api/core/v1"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"

	"github.com/kubernetes-sigs/sig-storage-lib-external-provisioner/controller"
)

const (
	csiParameterPrefix                    = "csi.storage.k8s.io/"
	prefixedProvisionerSecretNameKey      = csiParameterPrefix + "provisioner-secret-name"
	prefixedProvisionerSecretNamespaceKey = csiParameterPrefix + "provisioner-secret-namespace"
)

// rookCSIProvisioner struct
type rookCSIProvisioner struct {
	context        *clusterd.Context
	csiProvisioner controller.Provisioner
}

func newRookCSIProvisioner(context *clusterd.Context, csiProvisioner controller.Provisioner) controller.Provisioner {
	return &rookCSIProvisioner{
		context:        context,
		csiProvisioner: csiProvisioner,
	}
}

func (p *rookCSIProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {
	// extract Rook parameters and resolve them and inject into ceph-csi options
	params := make(map[string]string)
	for k, v := range options.Parameters {
		switch strings.ToLower(k) {
		case "clusternamespace":
			clusterInfo, _, _, _ := mon.LoadClusterInfo(p.context, v)
			monEndpoints := make([]string, 0, len(clusterInfo.Monitors))
			for _, monitor := range clusterInfo.Monitors {
				monEndpoints = append(monEndpoints, monitor.Endpoint)
			}
			params["monitors"] = strings.Join(monEndpoints, ",")
			params["adminid"] = "admin"
			params["userid"] = "admin"
			params[prefixedProvisionerSecretNamespaceKey] = v
			params[prefixedProvisionerSecretNameKey] = v + "-mon"
		case "blockpool":
			params["pool"] = v
		}
	}
	options.Parameters = params
	// pass to ceph-csi provision
	logger.Infof("provision params %#v", options.Parameters)
	return p.csiProvisioner.Provision(options)
}

func (p *rookCSIProvisioner) Delete(volume *v1.PersistentVolume) error {
	return p.csiProvisioner.Delete(volume)
}

func (p *rookCSIProvisioner) SupportsBlock() bool {
	return true
}
