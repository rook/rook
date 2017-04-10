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

	"github.com/rook/rook/pkg/operator/cluster"
	"github.com/rook/rook/pkg/operator/k8sutil"
	rookclient "github.com/rook/rook/pkg/rook/client"
	kwatch "k8s.io/client-go/pkg/watch"
)

type clusterManager struct {
	context      *context
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

func newClusterManager(context *context, inclusterInitiators []inclusterInitiator) *clusterManager {
	return &clusterManager{
		context:             context,
		clusters:            make(map[string]*cluster.Cluster),
		tracker:             newTPRTracker(),
		inclusterInitiators: inclusterInitiators,
	}
}

func (t *clusterManager) Name() string {
	return "rookcluster"
}

func (t *clusterManager) Description() string {
	return "Managed Rook clusters"
}

func (t *clusterManager) Manage() {
	for {
		logger.Infof("Managing clusters")
		err := t.Load()
		if err != nil {
			logger.Errorf("failed to load cluster. %+v", err)
		} else {
			if err := t.Watch(); err != nil {
				logger.Errorf("failed to watch clusters. %+v", err)
			}
		}

		<-time.After(time.Second * time.Duration(t.context.retryDelay))
	}
}

func (t *clusterManager) Load() error {

	// Check if there is an existing cluster to recover
	logger.Info("finding existing clusters...")
	clusterList, err := t.getClusterList()
	if err != nil {
		return err
	}
	logger.Infof("found %d clusters", len(clusterList.Items))
	for i := range clusterList.Items {
		c := clusterList.Items[i]
		t.startCluster(&c)
	}

	t.watchVersion = clusterList.Metadata.ResourceVersion
	return nil
}

func (t *clusterManager) startTrack(c *cluster.Cluster) {
	t.Lock()
	defer t.Unlock()

	// refresh the version of the cluster we're tracking
	t.tracker.add(c.Name, c.ResourceVersion)

	// only start the cluster if we're not already tracking it from a previous iteration
	if _, ok := t.clusters[c.Name]; !ok {
		t.clusters[c.Name] = c
	}
}

func (t *clusterManager) stopTrack(c *cluster.Cluster) {
	t.Lock()
	defer t.Unlock()

	t.tracker.remove(c.Name)
	delete(t.clusters, c.Name)
}

func (t *clusterManager) startCluster(c *cluster.Cluster) {
	c.Init(t.context.factory, t.context.clientset)
	t.startTrack(c)

	if t.devicesInUse && c.Spec.Storage.AnyUseAllDevices() {
		logger.Warningf("devices in more than one namespace not supported. ignoring devices in namespace %s", c.Namespace)
		c.Spec.Storage.ClearUseAllDevices()
	}

	if c.Spec.Storage.AnyUseAllDevices() {
		t.devicesInUse = true
	}

	go func() {
		defer t.stopTrack(c)

		// Start the Rook cluster components
		err := c.CreateInstance()
		if err != nil {
			logger.Errorf("failed to create cluster in namespace %s. %+v", c.Name, err)
			return
		}

		// Start all the TPRs for this cluster
		for _, tpr := range t.inclusterInitiators {
			k8sutil.Retry(time.Duration(t.context.retryDelay)*time.Second, t.context.maxRetries, func() (bool, error) {
				tprMgr, err := tpr.Create(t, c.Name)
				if err != nil {
					logger.Warningf("cannot create %s in-cluster tpr %s. %+v. retrying...", t.Name(), err)
					return false, nil
				}

				go tprMgr.Manage()

				// Start the tpr-manager asynchronously
				t.Lock()
				defer t.Unlock()
				t.inclusterMgrs = append(t.inclusterMgrs, tprMgr)

				return true, nil
			})
		}
		c.Monitor(t.tracker.stopChMap[c.Name])
	}()
}

func (t *clusterManager) isClustersCacheStale(currentClusters []cluster.Cluster) bool {
	if len(t.tracker.clusterRVs) != len(currentClusters) {
		return true
	}

	for _, cc := range currentClusters {
		rv, ok := t.tracker.clusterRVs[cc.Name]
		if !ok || rv != cc.ResourceVersion {
			return true
		}
	}

	return false
}

func (t *clusterManager) getClusterList() (*cluster.ClusterList, error) {
	b, err := getRawList(t.context.clientset, t.Name(), t.context.namespace)
	if err != nil {
		return nil, err
	}

	clusters := &cluster.ClusterList{}
	if err := json.Unmarshal(b, clusters); err != nil {
		return nil, err
	}
	return clusters, nil
}

func (t *clusterManager) Watch() error {
	logger.Infof("start watching cluster tpr: %s", t.watchVersion)
	defer t.tracker.stop()

	eventCh, errCh := t.watch()

	go func() {
		timer := k8sutil.NewPanicTimer(
			time.Minute,
			"unexpected long blocking (> 1 Minute) when handling cluster event")

		for event := range eventCh {
			timer.Start()

			c := event.Object

			switch event.Type {
			case kwatch.Added:

				logger.Infof("starting new cluster in namespace %s", c.Name)
				t.startCluster(c)

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
func (t *clusterManager) watch() (<-chan *clusterEvent, <-chan error) {
	eventCh := make(chan *clusterEvent)
	// On unexpected error case, the operator should exit
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)

		for {
			err := t.watchOuterTPR(eventCh, errCh)
			if err != nil {
				logger.Warningf("cancelling cluster tpr watch. %+v", err)
				return
			}
		}
	}()

	return eventCh, errCh
}

func (t *clusterManager) watchOuterTPR(eventCh chan *clusterEvent, errCh chan error) error {
	resp, err := watchTPR(t.context, t.Name(), t.context.namespace, t.watchVersion)
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
		done, err := handlePollEventResult(st, err, t.checkStaleCache, errCh)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		logger.Debugf("rook cluster event: %+v", ev)

		t.watchVersion = ev.Object.ResourceVersion
		eventCh <- ev
	}
}

func (t *clusterManager) checkStaleCache() (bool, error) {
	clusterList, err := t.getClusterList()
	if err == nil && !t.isClustersCacheStale(clusterList.Items) {
		t.watchVersion = clusterList.Metadata.ResourceVersion
		return false, nil
	}

	return true, err
}

func (t *clusterManager) getRookClient(namespace string) (rookclient.RookRestClient, error) {
	t.Lock()
	defer t.Unlock()
	if c, ok := t.clusters[namespace]; ok {
		return c.GetRookClient()
	}

	return nil, fmt.Errorf("namespace %s not found", namespace)
}
