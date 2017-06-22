/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/
package k8sutil

import (
	"fmt"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-k8sutil")

const (
	Namespace        = "rook"
	DefaultNamespace = "default"
	DataDirVolume    = "rook-data"
	DataDir          = "/var/lib/rook"
	RookType         = "kubernetes.io/rook"
	RbdType          = "kubernetes.io/rbd"
)

type ConditionFunc func() (bool, error)

// Retry retries f every interval until after maxRetries.
// The interval won't be affected by how long f takes.
// For example, if interval is 3s, f takes 1s, another f will be called 2s later.
// However, if f takes longer than interval, it will be delayed.
func Retry(context *clusterd.Context, f ConditionFunc) error {
	interval := time.Duration(context.RetryDelay) * time.Second
	tick := time.NewTicker(interval)
	defer tick.Stop()

	for i := 0; i < context.MaxRetries; i++ {
		ok, err := f()
		if err != nil {
			return fmt.Errorf("failed on retry %d. %+v", i, err)
		}
		if ok {
			return nil
		}
		if i < context.MaxRetries-1 {
			<-tick.C
		}
	}
	return fmt.Errorf("failed after max retries %d.", context.MaxRetries)
}
