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
	"github.com/rook/rook/pkg/operator/ceph/cluster/crash"
	"github.com/rook/rook/pkg/operator/ceph/pool"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager (entrypoint for controller)
var AddToManagerFuncs = []func(manager.Manager, *clusterd.Context) error{
	crash.Add,
	pool.Add,
}

// AddToManager adds all the registered controllers to the passed manager.
// each controller package will have an Add method listed in AddToManagerFuncs
// which will setup all the necessary watch
func AddToManager(m manager.Manager, c *clusterd.Context) error {
	if c == nil {
		return errors.New("nil context passed")
	}

	for _, f := range AddToManagerFuncs {
		if err := f(m, c); err != nil {
			return err
		}
	}

	return nil
}
