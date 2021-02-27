/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	scheme    = runtime.NewScheme()
	resources = []webhook.Validator{&cephv1.CephCluster{}, &cephv1.CephBlockPool{}, &cephv1.CephObjectStore{}}
)

const (
	// Default directory where TLS certs are stored
	certDir = "/etc/webhook"
	// Default port for server
	port = 8079
)

// StartAdmissionController will start the server
func StartAdmissionController() error {
	logger.Infof("starting the webhook for backend ceph")
	err := cephv1.AddToScheme(scheme)
	if err != nil {
		return errors.Wrap(err, "failed to add to scheme")
	}
	opts := ctrl.Options{
		Scheme:  scheme,
		Port:    port,
		CertDir: certDir,
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), opts)
	if err != nil {
		return errors.Wrap(err, "failed to create manager")
	}
	for _, resource := range resources {
		err = ctrl.NewWebhookManagedBy(mgr).For(resource).Complete()
		if err != nil {
			return errors.Wrap(err, "failed to register webhooks")
		}
	}
	logger.Info("starting webhook server")
	err = mgr.Start(ctrl.SetupSignalHandler())
	if err != nil {
		return errors.Wrap(err, "failed to start server")
	}

	return nil
}
