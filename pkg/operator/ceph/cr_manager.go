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
	"github.com/rook/rook/pkg/operator/ceph/cluster/crash"
	"github.com/rook/rook/pkg/operator/ceph/cluster/rbd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/ceph/disruption/clusterdisruption"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	"github.com/rook/rook/pkg/operator/ceph/disruption/machinedisruption"
	"github.com/rook/rook/pkg/operator/ceph/disruption/machinelabel"
	"github.com/rook/rook/pkg/operator/ceph/file"
	"github.com/rook/rook/pkg/operator/ceph/file/mirror"
	"github.com/rook/rook/pkg/operator/ceph/file/subvolumegroup"
	"github.com/rook/rook/pkg/operator/ceph/nfs"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/ceph/object/bucket"
	"github.com/rook/rook/pkg/operator/ceph/object/notification"
	"github.com/rook/rook/pkg/operator/ceph/object/realm"
	"github.com/rook/rook/pkg/operator/ceph/object/topic"
	objectuser "github.com/rook/rook/pkg/operator/ceph/object/user"
	"github.com/rook/rook/pkg/operator/ceph/object/zone"
	"github.com/rook/rook/pkg/operator/ceph/object/zonegroup"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	"github.com/rook/rook/pkg/operator/ceph/pool/radosnamespace"
	"k8s.io/apimachinery/pkg/runtime"

	mapiv1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	healthchecking "github.com/openshift/machine-api-operator/pkg/apis/healthchecking/v1alpha1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	certDir = "/etc/webhook"
)

var (
	resourcesSchemeFuncs = []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		mapiv1.AddToScheme,
		healthchecking.AddToScheme,
		cephv1.AddToScheme,
	}
)

var (
	webhookResources = []webhook.Validator{
		&cephv1.CephCluster{},
		&cephv1.CephBlockPool{},
		&cephv1.CephObjectStore{},
		&cephv1.CephBlockPoolRadosNamespace{},
		&cephv1.CephFilesystemSubVolumeGroup{},
	}
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
var AddToManagerFuncs = []func(manager.Manager, *clusterd.Context, context.Context, opcontroller.OperatorConfig) error{
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
	Add,
	csi.Add,
	bucket.Add,
	topic.Add,
	notification.Add,
	subvolumegroup.Add,
	radosnamespace.Add,
}

// AddToManagerOpFunc is a list of functions to add all Controllers to the Manager (entrypoint for
// controller)
// var AddToManagerOpFunc = []func(manager.Manager, *clusterd.Context, opcontroller.OperatorConfig) error{}

// AddToManager adds all the registered controllers to the passed manager.
// each controller package will have an Add method listed in AddToManagerFuncs
// which will setup all the necessary watch
func (o *Operator) addToManager(m manager.Manager, c *controllerconfig.Context, opManagerContext context.Context) error {
	if c == nil {
		return errors.New("nil context passed")
	}

	// Run CephCluster CR
	if err := cluster.Add(m, c.ClusterdContext, o.clusterController, opManagerContext); err != nil {
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

	// Set up a manager
	mgrOpts := manager.Options{
		LeaderElection: false,
		Namespace:      o.config.NamespaceToWatch,
		Scheme:         scheme,
		CertDir:        certDir,
	}

	logger.Info("setting up the controller-runtime manager")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to set up overall controller-runtime manager")
		return
	}

	// Add webhook if needed
	isPresent, err := createWebhook(context, o.context)
	if err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to retrieve admission webhook secret")
		return
	}
	if isPresent {
		err := createWebhookService(context, o.context)
		if err != nil {
			mgrErrorCh <- errors.Wrap(err, "failed to create admission webhook service")
			return
		}
		logger.Info("setting up admission webhooks")
		for _, resource := range webhookResources {
			err = ctrl.NewWebhookManagedBy(mgr).For(resource).Complete()
			if err != nil {
				mgrErrorCh <- errors.Wrapf(err, "failed to register webhook for %q", resource.GetObjectKind().GroupVersionKind().Kind)
				return
			}
		}
	}

	// options to pass to the controllers
	controllerOpts := &controllerconfig.Context{
		ClusterdContext:   o.context,
		ReconcileCanaries: &controllerconfig.LockingBool{},
		OpManagerContext:  context,
	}

	// Add the registered controllers to the manager (entrypoint for controllers)
	err = o.addToManager(mgr, controllerOpts, context)
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
