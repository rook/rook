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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	"k8s.io/apimachinery/pkg/runtime"

	mapiv1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	healthchecking "github.com/openshift/machine-api-operator/pkg/apis/healthchecking/v1alpha1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	resourcesSchemeFuncs = []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		mapiv1.AddToScheme,
		healthchecking.AddToScheme,
		cephv1.AddToScheme,
		v1alpha2.AddToScheme,
	}
)

func (o *Operator) startManager(namespaceToWatch string, context context.Context, mgrErrorCh chan error) {
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
		Namespace:      namespaceToWatch,
		Scheme:         scheme,
	}

	logger.Info("setting up the controller-runtime manager")
	kubeConfig, err := config.GetConfig()
	if err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to get client config for controller-runtime manager")
		return
	}

	mgr, err := manager.New(kubeConfig, mgrOpts)
	if err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to set up overall controller-runtime manager")
		return
	}

	// options to pass to the controllers
	controllerOpts := &controllerconfig.Context{
		RookImage:         o.rookImage,
		ClusterdContext:   o.context,
		OperatorNamespace: o.operatorNamespace,
		ReconcileCanaries: &controllerconfig.LockingBool{},
	}
	// Add the registered controllers to the manager (entrypoint for controllers)
	err = cluster.AddToManager(mgr, controllerOpts, o.clusterController)
	if err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to add controllers to controller-runtime manager")
		return
	}

	logger.Info("starting the controller-runtime manager")
	if err := mgr.Start(context); err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to run the controller-runtime manager")
		return
	}
}
