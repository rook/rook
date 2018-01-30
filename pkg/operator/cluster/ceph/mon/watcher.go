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

// Package mon for the Ceph monitors.
package mon

import (
	"fmt"

	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
)

// EndpointWatcher check health for the monitors
type EndpointWatcher struct {
	monCluster *Cluster
}

// NewEndpointWatcher creates a new EndpointWatcher object
func NewEndpointWatcher(monCluster *Cluster) *EndpointWatcher {
	return &EndpointWatcher{
		monCluster: monCluster,
	}
}

// StartWatch watch the mon pods for IP changes
func (hc *EndpointWatcher) StartWatch(stopCh chan struct{}) {
	// "infinite" loop to keep watching forever until stopped, this function
	// should be called in a goroutine
	for {
		// watch for changes only to the monitor endpoints config map
		opts := metav1.ListOptions{
			LabelSelector: labels.FormatLabels(hc.monCluster.getLabels()),
		}
		w, err := hc.monCluster.context.Clientset.Core().Pods(hc.monCluster.Namespace).Watch(opts)
		if err != nil {
			logger.Errorf("WatchMonConfig watch init error: %+v", err)
			return
		}
		defer w.Stop()

		select {
		case <-stopCh:
			logger.Infof("stopping mon endpoint watcher of cluster in namespace %s", hc.monCluster.Namespace)
			return
		case e, ok := <-w.ResultChan():
			if !ok {
				logger.Warning("EndpointWatcher.StartWatch result channel closed, restarting watch.")
				w.Stop()
				break
			}

			if e.Type == watch.Modified {
				// cast object into Pod and update mon endpoint IP if changed
				updated := e.Object.(*v1.Pod)
				current := hc.monCluster.clusterInfo.Monitors[updated.Name]

				hc.compareAndUpdateMonEndpointFromPod(current, updated)
			}
		}
	}
}

func (hc *EndpointWatcher) compareAndUpdateMonEndpointFromPod(current *mon.CephMonitorConfig, updated *v1.Pod) {
	if current.Endpoint != fmt.Sprintf("%s:%d", updated.Status.PodIP, mon.DefaultPort) {
		logger.Debugf("mon %s Pod IP change (current: %s, new: %s)",
			updated.Name,
			hc.monCluster.clusterInfo.RemovePortFromEndpoint(current.Endpoint),
			updated.Status.PodIP)
		hc.monCluster.clusterInfo.MonMutex.Lock()
		current = mon.ToCephMon(updated.Name, updated.Status.PodIP, mon.DefaultPort)
		hc.monCluster.clusterInfo.MonMutex.Unlock()

		// reading access to maps doesn't require lock
		if err := hc.monCluster.saveConfigChanges(); err != nil {
			logger.Errorf("failed to save mons. %+v", err)
		}
	} else {
		logger.Debugf("no change for mon %s Pod IP", updated.Name)
	}
}
