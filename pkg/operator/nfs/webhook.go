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

package nfs

import (
	nfsv1alpha1 "github.com/rook/rook/pkg/apis/nfs.rook.io/v1alpha1"

	ctrl "sigs.k8s.io/controller-runtime"
)

type Webhook struct {
	Port    int
	CertDir string
}

func NewWebhook(port int, certDir string) *Webhook {
	return &Webhook{
		Port:    port,
		CertDir: certDir,
	}
}

func (w *Webhook) Run() error {
	opts := ctrl.Options{
		Port:   w.Port,
		Scheme: scheme,
	}

	if w.CertDir != "" {
		opts.CertDir = w.CertDir
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), opts)
	if err != nil {
		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&nfsv1alpha1.NFSServer{}).
		Complete(); err != nil {
		return err
	}

	logger.Info("starting webhook manager")
	return mgr.Start(ctrl.SetupSignalHandler())
}
