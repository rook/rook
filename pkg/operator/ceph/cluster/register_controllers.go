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

package cluster

import (
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/crash"
	"github.com/rook/rook/pkg/operator/ceph/cluster/rbd"
	"github.com/rook/rook/pkg/operator/ceph/disruption/clusterdisruption"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	"github.com/rook/rook/pkg/operator/ceph/disruption/machinedisruption"
	"github.com/rook/rook/pkg/operator/ceph/disruption/machinelabel"
	"github.com/rook/rook/pkg/operator/ceph/file"
	"github.com/rook/rook/pkg/operator/ceph/file/mirror"
	"github.com/rook/rook/pkg/operator/ceph/nfs"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/ceph/object/realm"
	objectuser "github.com/rook/rook/pkg/operator/ceph/object/user"
	"github.com/rook/rook/pkg/operator/ceph/object/zone"
	"github.com/rook/rook/pkg/operator/ceph/object/zonegroup"
	"github.com/rook/rook/pkg/operator/ceph/pool"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	// EnableMachineDisruptionBudget checks whether machine disruption budget is enabled
	EnableMachineDisruptionBudget bool
)

// AddToManagerFuncsMaintenance is a list of functions to add all Controllers to the Manager (entrypoint for controller)
var AddToManagerFuncsMaintenance = []func(manager.Manager, *controllerconfig.Context) error{
	clusterdisruption.Add,
}

// MachineDisruptionBudgetAddToManagerFuncs is a list of fencing related functions to add all Controllers to the Manager (entrypoint for controller)
var MachineDisruptionBudgetAddToManagerFuncs = []func(manager.Manager, *controllerconfig.Context) error{
	machinelabel.Add,
	machinedisruption.Add,
}

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager (entrypoint for controller)
var AddToManagerFuncs = []func(manager.Manager, *clusterd.Context) error{
	crash.Add,
	pool.Add,
	objectuser.Add,
	realm.Add,
	zonegroup.Add,
	zone.Add,
	object.Add,
	file.Add,
	nfs.Add,
	rbd.Add,
	client.Add,
	mirror.Add,
}

// AddToManager adds all the registered controllers to the passed manager.
// each controller package will have an Add method listed in AddToManagerFuncs
// which will setup all the necessary watch
func AddToManager(m manager.Manager, c *controllerconfig.Context, clusterController *ClusterController) error {
	if c == nil {
		return errors.New("nil context passed")
	}

	// Run CephCluster CR
	if err := Add(m, c.ClusterdContext, clusterController); err != nil {
		return err
	}

	// Add Ceph child CR controllers
	for _, f := range AddToManagerFuncs {
		if err := f(m, c.ClusterdContext); err != nil {
			return err
		}
	}

	// Add maintenance controllers
	for _, f := range AddToManagerFuncsMaintenance {
		if err := f(m, c); err != nil {
			return err
		}
	}

	// If machine disruption budget is enabled let's add the controllers
	if EnableMachineDisruptionBudget {
		for _, f := range MachineDisruptionBudgetAddToManagerFuncs {
			if err := f(m, c); err != nil {
				return err
			}
		}
	}

	return nil
}
