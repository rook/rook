/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package nfs operator to manage NFS Server.
package nfs

import (
	nfsv1alpha1 "github.com/rook/rook/pkg/apis/nfs.rook.io/v1alpha1"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	scheme         = runtime.NewScheme()
	controllerName = "nfs-operator"
	logger         = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)
)

// Operator type for managing NFS Server.
type Operator struct {
	context *clusterd.Context
}

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = nfsv1alpha1.AddToScheme(scheme)
}

// New creates an operator instance.
func New(context *clusterd.Context) *Operator {
	return &Operator{
		context: context,
	}
}

// Run the operator instance.
func (o *Operator) Run() error {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		return err
	}

	reconciler := &NFSServerReconciler{
		Client:   mgr.GetClient(),
		Context:  o.context,
		Log:      logger,
		Scheme:   scheme,
		Recorder: mgr.GetEventRecorderFor(controllerName),
	}

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&nfsv1alpha1.NFSServer{}).
		Complete(reconciler); err != nil {
		return err
	}

	logger.Info("starting manager")
	return mgr.Start(ctrl.SetupSignalHandler())
}
