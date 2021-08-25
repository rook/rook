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
	"github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
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
		v1alpha2.AddToScheme,
	}
)

var (
	webhookResources = []webhook.Validator{&cephv1.CephCluster{}, &cephv1.CephBlockPool{}, &cephv1.CephObjectStore{}}
)

func (o *Operator) startManager(context context.Context, namespaceToWatch string, mgrErrorCh chan error) {
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
		CertDir:        certDir,
	}

	logger.Info("setting up the controller-runtime manager")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to set up overall controller-runtime manager")
		return
	}

	// options to pass to the controllers
	controllerOpts := &controllerconfig.Context{
		RookImage:         o.config.Image,
		ClusterdContext:   o.context,
		OperatorNamespace: o.config.OperatorNamespace,
		ReconcileCanaries: &controllerconfig.LockingBool{},
	}

	// Add the registered controllers to the manager (entrypoint for controllers)
	err = cluster.AddToManager(mgr, controllerOpts, o.clusterController)
	if err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to add controllers to controller-runtime manager")
		return
	}

	// Add Webhooks
	// TODO: this needs a callback with a watcher for this secret so we can quickly enable the webhook if it changes
	isPresent, err := isSecretPresent(context, o.context)
	if err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to retrieve admission webhook secret")
		return
	}
	if isPresent {
		err := createWebhookService(o.context)
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

	logger.Info("starting the controller-runtime manager")
	if err := mgr.Start(context); err != nil {
		mgrErrorCh <- errors.Wrap(err, "failed to run the controller-runtime manager")
		return
	}
}
