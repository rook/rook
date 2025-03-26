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

package operator

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	"github.com/rook/rook/pkg/operator/ceph/cluster/nodedaemon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/rbd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/ceph/disruption/clusterdisruption"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	"github.com/rook/rook/pkg/operator/ceph/file"
	"github.com/rook/rook/pkg/operator/ceph/file/mirror"
	"github.com/rook/rook/pkg/operator/ceph/file/subvolumegroup"
	"github.com/rook/rook/pkg/operator/ceph/nfs"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/ceph/object/bucket"
	"github.com/rook/rook/pkg/operator/ceph/object/cosi"
	"github.com/rook/rook/pkg/operator/ceph/object/notification"
	"github.com/rook/rook/pkg/operator/ceph/object/realm"
	"github.com/rook/rook/pkg/operator/ceph/object/topic"
	objectuser "github.com/rook/rook/pkg/operator/ceph/object/user"
	"github.com/rook/rook/pkg/operator/ceph/object/zone"
	"github.com/rook/rook/pkg/operator/ceph/object/zonegroup"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	"github.com/rook/rook/pkg/operator/ceph/pool/radosnamespace"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/apimachinery/pkg/runtime"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var resourcesSchemeFuncs = []func(*runtime.Scheme) error{
	clientgoscheme.AddToScheme,
	cephv1.AddToScheme,
}

// EnableMachineDisruptionBudget checks whether machine disruption budget is enabled
var EnableMachineDisruptionBudget bool

// AddToManagerFuncsMaintenance is a list of functions to add all Controllers to the Manager (entrypoint for controller)
var AddToManagerFuncsMaintenance = []func(manager.Manager, *controllerconfig.Context) error{
	clusterdisruption.Add,
}

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager (entrypoint for controller)
var AddToManagerFuncs = []func(manager.Manager, *clusterd.Context, context.Context, opcontroller.OperatorConfig) error{
	nodedaemon.Add,
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
	Add,
	csi.Add,
	bucket.Add,
	topic.Add,
	notification.Add,
	subvolumegroup.Add,
	radosnamespace.Add,
	cosi.Add,
}

// AddToManagerOpFunc is a list of functions to add all Controllers to the Manager (entrypoint for
// controller)
// var AddToManagerOpFunc = []func(manager.Manager, *clusterd.Context, opcontroller.OperatorConfig) error{}

// AddToManager adds all the registered controllers to the passed manager.
// each controller package will have an Add method listed in AddToManagerFuncs
// which will setup all the necessary watch
func (o *Operator) addToManager(m manager.Manager, c *controllerconfig.Context, opManagerContext context.Context, opconfig opcontroller.OperatorConfig) error {
	if c == nil {
		return errors.New("nil context passed")
	}

	// Run CephCluster CR
	if err := cluster.Add(m, c.ClusterdContext, o.clusterController, opManagerContext, opconfig); err != nil {
		return err
	}

	// Add Ceph child CR controllers
	for _, f := range AddToManagerFuncs {
		if err := f(m, c.ClusterdContext, opManagerContext, *o.config); err != nil {
			return err
		}
	}

	// Add maintenance controllers
	for _, f := range AddToManagerFuncsMaintenance {
		if err := f(m, c); err != nil {
			return err
		}
	}

	return nil
}

func (o *Operator) startCRDManager(context context.Context, mgrErrorCh chan error) {
	logger.Info("setting up schemes")
	// Setup Scheme for all resources
	scheme := runtime.NewScheme()
	for _, f := range resourcesSchemeFuncs {
		err := f(scheme)
		if err != nil {
			mgrErrorCh <- errors.Wrap(err, "failed to add to scheme")
			return
		}
	}

	metricsBindAddress := k8sutil.GetOperatorSetting("ROOK_OPERATOR_METRICS_BIND_ADDRESS", "0")
	skipNameValidation := true
	// Set up a manager
	mgrOpts := manager.Options{
		LeaderElection: false,
		Metrics: metricsserver.Options{
			// BindAddress is the bind address for controller runtime metrics server. Defaulted to "0" which is off.
			BindAddress: metricsBindAddress,
		},
		Scheme: scheme,
		Controller: config.Controller{
			SkipNameValidation: &skipNameValidation,
		},
	}

	if o.config.NamespaceToWatch != "" {
		mgrOpts.Cache = cache.Options{
			DefaultNamespaces: map[string]cache.Config{o.config.NamespaceToWatch: {}},
		}
	}

	logger.Info("setting up the controller-runtime manager")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to set up overall controller-runtime manager")
		return
	}

	// options to pass to the controllers
	controllerOpts := &controllerconfig.Context{
		ClusterdContext:   o.context,
		ReconcileCanaries: &controllerconfig.LockingBool{},
		OpManagerContext:  context,
	}

	// Add the registered controllers to the manager (entrypoint for controllers)
	err = o.addToManager(mgr, controllerOpts, context, *o.config)
	if err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to add controllers to controller-runtime manager")
		return
	}

	logger.Info("starting the controller-runtime manager")
	if err := mgr.Start(context); err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to run the controller-runtime manager")
		return
	}

	logger.Info("successfully started the controller-runtime manager")
}
