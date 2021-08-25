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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"os"
	"reflect"

	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
)

// StartOperatorSettingsWatch starts the operator settings watcher
// TODO: use controller runtime, so we can use a Context to cancel the watch instead of using another channel
// The cache package mentions that it'd be nice to use a Context for cancellation too
func (c *ClusterController) StartOperatorSettingsWatch(stopCh chan struct{}) {
	operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	// watch for "rook-ceph-operator-config" ConfigMap
	k8sutil.StartOperatorSettingsWatch(c.context, operatorNamespace, opcontroller.OperatorSettingConfigMapName,
		c.operatorConfigChange,
		func(oldObj, newObj interface{}) {
			if reflect.DeepEqual(oldObj, newObj) {
				return
			}
			c.operatorConfigChange(newObj)
		}, nil, stopCh)
}

// StopWatch stop watchers
func (c *ClusterController) StopWatch() {
	for _, cluster := range c.clusterMap {
		// check channel is open before closing
		if !cluster.closedStopCh {
			close(cluster.stopCh)
			cluster.closedStopCh = true
		}
	}
	c.clusterMap = make(map[string]*cluster)
}

func (c *ClusterController) operatorConfigChange(obj interface{}) {
	cm, ok := obj.(*v1.ConfigMap)
	if !ok {
		logger.Warningf("Expected ConfigMap but handler received %T. %#v", obj, obj)
		return
	}

	logger.Infof("ConfigMap %q changes detected. Updating configurations", cm.Name)
	for _, callback := range c.operatorConfigCallbacks {
		if err := callback(); err != nil {
			logger.Errorf("%v", err)
		}
	}
}
