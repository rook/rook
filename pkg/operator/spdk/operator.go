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

package spdk

import (
	"github.com/coreos/pkg/capnslog"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	spdkv1alpha1 "github.com/rook/rook/pkg/apis/spdk.rook.io/v1alpha1"
)

var (
	scheme = runtime.NewScheme()
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "spdk-operator")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = spdkv1alpha1.AddToScheme(scheme)
}

// Run the operator instance.
func Run() error {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		return err
	}

	err = (&ClusterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    logger,
	}).SetupWithManager(mgr)
	if err != nil {
		return err
	}

	logger.Info("starting manager")
	return mgr.Start(ctrl.SetupSignalHandler())
}
