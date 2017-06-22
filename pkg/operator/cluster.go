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
package operator

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"sync"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/cluster"
	"github.com/rook/rook/pkg/operator/k8sutil"
	rookclient "github.com/rook/rook/pkg/rook/client"
	kwatch "k8s.io/apimachinery/pkg/watch"
)

type clusterManager struct {
	context      *clusterd.Context
	name         string
	watchVersion string
	devicesInUse bool
	clusters     map[string]*cluster.Cluster
	tracker      *tprTracker
	sync.RWMutex
	// The initiators that create TPRs specific to specific Rook clusters.
	// For example, pools, object services, and file services, only make sense in the context of a Rook cluster
	inclusterInitiators []inclusterInitiator
	inclusterMgrs       []tprManager
}

func newClusterManager(context *clusterd.Context, inclusterInitiators []inclusterInitiator) *clusterManager {
	return &clusterManager{
		context:             context,
		clusters:            make(map[string]*cluster.Cluster),
		tracker:             newTPRTracker(),
		inclusterInitiators: inclusterInitiators,
	}
}

// Gets the name of the TPR
func (m *clusterManager) Name() string {
	return "cluster"
}

// Gets the description of the TPR
func (m *clusterManager) Description() string {
	return "Managed Rook clusters"
}

func (m *clusterManager) Manage() {
	for {
		logger.Infof("Managing clusters")
		err := m.Load()
		if err != nil {
			logger.Errorf("failed to load cluster. %+v", err)
		} else {
			if err := m.Watch(); err != nil {
				logger.Errorf("failed to watch clusters. %+v", err)
			}
		}

		<-time.After(time.Second * time.Duration(m.context.RetryDelay))
	}
}

func (m *clusterManager) Load() error {

	// Check if there is an existing cluster to recover
	logger.Info("finding existing clusters...")
	clusterList, err := m.getClusterList()
	if err != nil {
		return err
	}
	logger.Infof("found %d clusters", len(clusterList.Items))
	for i := range clusterList.Items {
		c := clusterList.Items[i]
		logger.Infof("checking if cluster %s is running in namespace %s", c.Name, c.Namespace)
		m.startCluster(&c)
	}

	m.watchVersion = clusterList.Metadata.ResourceVersion
	return nil
}

func (m *clusterManager) startTrack(c *cluster.Cluster) error {
	m.Lock()
	defer m.Unlock()

	existing, ok := m.clusters[c.Namespace]
	if ok {
		if c.Name != existing.Name {
			return fmt.Errorf("cluster %s is already running in namespace %s. Multiple clusters per namespace not supported.", existing.Name, existing.Namespace)
		}
	} else {
		// only start the cluster if we're not already tracking it from a previous iteration
		m.clusters[c.Namespace] = c
	}

	// refresh the version of the cluster we're tracking
	m.tracker.add(c.Namespace, c.ResourceVersion)

	return nil
}

func (m *clusterManager) stopTrack(c *cluster.Cluster) {
	m.Lock()
	defer m.Unlock()

	m.tracker.remove(c.Namespace)
	delete(m.clusters, c.Namespace)
}

func (m *clusterManager) startCluster(c *cluster.Cluster) {
	c.Init(m.context)
	if err := m.startTrack(c); err != nil {
		logger.Errorf("failed to start cluster %s in namespace %s. %+v", c.Name, c.Namespace, err)
		return
	}

	if m.devicesInUse && c.Spec.Storage.AnyUseAllDevices() {
		logger.Warningf("using all devices in more than one namespace not supported. ignoring devices in namespace %s", c.Namespace)
		c.Spec.Storage.ClearUseAllDevices()
	}

	if c.Spec.Storage.AnyUseAllDevices() {
		m.devicesInUse = true
	}

	go func() {
		defer m.stopTrack(c)
		logger.Infof("starting cluster %s in namespace %s", c.Name, c.Namespace)

		// Start the Rook cluster components. Retry several times in case of failure.
		err := k8sutil.Retry(m.context, func() (bool, error) {
			err := c.CreateInstance()
			if err != nil {
				logger.Errorf("failed to create cluster %s in namespace %s. %+v", c.Name, c.Namespace, err)
				return false, nil
			}
			return true, nil
		})
		if err != nil {
			logger.Errorf("giving up to create cluster %s in namespace %s", c.Name, c.Namespace)
			return
		}

		// Start all the TPRs for this cluster
		for _, tpr := range m.inclusterInitiators {
			k8sutil.Retry(m.context, func() (bool, error) {
				tprMgr, err := tpr.Create(m, c.Name, c.Namespace)
				if err != nil {
					logger.Warningf("cannot create in-cluster tpr %s. %+v. retrying...", m.Name(), err)
					return false, nil
				}

				// Start the tpr-manager asynchronously
				go tprMgr.Manage()

				m.Lock()
				defer m.Unlock()
				m.inclusterMgrs = append(m.inclusterMgrs, tprMgr)

				return true, nil
			})
		}
		c.Monitor(m.tracker.stopChMap[c.Namespace])
	}()
}

func (m *clusterManager) isClustersCacheStale(currentClusters []cluster.Cluster) bool {
	if len(m.tracker.clusterRVs) != len(currentClusters) {
		return true
	}

	for _, cc := range currentClusters {
		rv, ok := m.tracker.clusterRVs[cc.Name]
		if !ok || rv != cc.ResourceVersion {
			return true
		}
	}

	return false
}

func (m *clusterManager) getClusterList() (*cluster.ClusterList, error) {
	b, err := getRawList(m.context.Clientset, m.Name())
	if err != nil {
		return nil, err
	}

	clusters := &cluster.ClusterList{}
	if err := json.Unmarshal(b, clusters); err != nil {
		return nil, err
	}
	return clusters, nil
}

func (m *clusterManager) Watch() error {
	logger.Infof("start watching cluster tpr: %s", m.watchVersion)
	defer m.tracker.stop()

	eventCh, errCh := m.watch()

	go func() {
		timer := k8sutil.NewPanicTimer(
			time.Minute,
			"unexpected long blocking (> 1 Minute) when handling cluster event")

		for event := range eventCh {
			timer.Start()

			c := event.Object

			switch event.Type {
			case kwatch.Added:
				logger.Infof("starting new cluster %s in namespace %s", c.Name, c.Namespace)
				m.startCluster(c)

			case kwatch.Modified:
				logger.Infof("modifying a cluster not implemented")

			case kwatch.Deleted:
				logger.Infof("deleting a cluster not implemented")
			}

			timer.Stop()
		}
	}()
	return <-errCh
}

// watch creates a go routine, and watches the cluster.rook kind resources from
// the given watch version. It emits events on the resources through the returned
// event chan. Errors will be reported through the returned error chan. The go routine
// exits on any error.
func (m *clusterManager) watch() (<-chan *clusterEvent, <-chan error) {
	eventCh := make(chan *clusterEvent)
	// On unexpected error case, the operator should exit
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)

		for {
			err := m.watchOuterTPR(eventCh, errCh)
			if err != nil {
				logger.Warningf("cancelling cluster tpr watch. %+v", err)
				return
			}
		}
	}()

	return eventCh, errCh
}

func (m *clusterManager) watchOuterTPR(eventCh chan *clusterEvent, errCh chan error) error {
	resp, err := watchTPR(m.context, m.Name(), m.watchVersion)
	if err != nil {
		errCh <- err
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := errors.New("invalid status code: " + resp.Status)
		errCh <- err
		return err
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		ev, st, err := pollClusterEvent(decoder)
		done, err := handlePollEventResult(st, err, m.checkStaleCache, errCh)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		logger.Debugf("rook cluster event: %+v", ev)

		m.watchVersion = ev.Object.ResourceVersion
		eventCh <- ev
	}
}

func (m *clusterManager) checkStaleCache() (bool, error) {
	clusterList, err := m.getClusterList()
	if err == nil && !m.isClustersCacheStale(clusterList.Items) {
		m.watchVersion = clusterList.Metadata.ResourceVersion
		return false, nil
	}

	return true, err
}

func (m *clusterManager) getRookClient(namespace string) (rookclient.RookRestClient, error) {
	m.Lock()
	defer m.Unlock()
	if c, ok := m.clusters[namespace]; ok {
		return c.GetRookClient()
	}

	return nil, fmt.Errorf("namespace %s not found", namespace)
}
