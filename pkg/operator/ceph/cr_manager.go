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
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	controllers "github.com/rook/rook/pkg/operator/ceph/disruption"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func (o *Operator) startManager(stopCh <-chan struct{}) {

	// Set up a manager
	mgrOpts := manager.Options{
		LeaderElection: false,
	}

	logger.Info("setting up the controller-runtime manager")
	kubeConfig, err := config.GetConfig()
	if err != nil {
		logger.Errorf("unable to get client config for controller-runtime manager. %v", err)
		return
	}
	mgr, err := manager.New(kubeConfig, mgrOpts)
	if err != nil {
		logger.Errorf("unable to set up overall controller-runtime manager. %v", err)
		return
	}

	// Add the registered controllers to the manager (entrypoint for controllers)
	err = cluster.AddToManager(mgr)
	if err != nil {
		logger.Errorf("cannot add controllers to controller-runtime manager. %v", err)
	}
	// options to pass to the controllers
	controllerOpts := &controllerconfig.Context{
		RookImage:         o.rookImage,
		ClusterdContext:   o.context,
		OperatorNamespace: o.operatorNamespace,
		ReconcileCanaries: &controllerconfig.LockingBool{},
	}
	// Add the registered controllers to the manager (entrypoint for controllers)
	err = controllers.AddToManager(mgr, controllerOpts)
	if err != nil {
		logger.Errorf("cannot add controllers to controller-runtime manager. %v", err)
	}

	logger.Info("starting the controller-runtime manager")
	if err := mgr.Start(stopCh); err != nil {
		logger.Errorf("unable to run the controller-runtime manager. %v", err)
		return
	}
}
