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

type clusterTPR struct {
	context      *context
	watchVersion string
	devicesInUse bool
	clusters     map[string]*cluster.Cluster
	tracker      *tprTracker
	sync.RWMutex
}

func newClusterTPR(context *context) *clusterTPR {
	return &clusterTPR{
		context:  context,
		clusters: make(map[string]*cluster.Cluster),
		tracker:  newTPRTracker(),
	}
}

func (t *clusterTPR) Name() string {
	return "rookcluster"
}

func (t *clusterTPR) Description() string {
	return "Managed Rook clusters"
}

func (t *clusterTPR) Load() error {

	// Check if there is an existing cluster to recover
	err := t.findAllClusters()
	if err != nil {
		return fmt.Errorf("failed to find clusters. %+v", err)
	}
	return nil
}

func (t *clusterTPR) findAllClusters() error {
	logger.Info("finding existing clusters...")
	clusterList, err := t.getClusterList()
	if err != nil {
		return err
	}
	logger.Infof("found %d clusters", len(clusterList.Items))
	for i := range clusterList.Items {
		c := clusterList.Items[i]

		ns := c.Spec.Namespace
		existingCluster := cluster.New(c.Spec, t.context.factory, t.context.clientset)
		t.tracker.add(ns, c.Metadata.ResourceVersion)
		t.clusters[ns] = existingCluster

		logger.Infof("resuming cluster %s in namespace %s", c.Metadata.Name, ns)
		t.startCluster(existingCluster)
	}

	t.watchVersion = clusterList.Metadata.ResourceVersion
	return nil
}

func (t *clusterTPR) startCluster(c *cluster.Cluster) {
	if t.devicesInUse && c.Spec.UseAllDevices {
		logger.Warningf("devices in more than one namespace not supported. ignoring devices in namespace %s", c.Spec.Namespace)
		c.Spec.UseAllDevices = false
	}

	if c.Spec.UseAllDevices {
		t.devicesInUse = true
	}

	go func() {
		err := c.CreateInstance()
		if err != nil {
			logger.Errorf("failed to create cluster in namespace %s. %+v", c.Spec.Namespace, err)
			return
		}
		c.Monitor(t.tracker.stopChMap[c.Spec.Namespace])
	}()
}

func (t *clusterTPR) isClustersCacheStale(currentClusters []cluster.Cluster) bool {
	if len(t.tracker.clusterRVs) != len(currentClusters) {
		return true
	}

	for _, cc := range currentClusters {
		rv, ok := t.tracker.clusterRVs[cc.Metadata.Name]
		if !ok || rv != cc.Metadata.ResourceVersion {
			return true
		}
	}

	return false
}

func (t *clusterTPR) getClusterList() (*cluster.ClusterList, error) {
	b, err := getRawList(t.context, t)
	if err != nil {
		return nil, err
	}

	clusters := &cluster.ClusterList{}
	if err := json.Unmarshal(b, clusters); err != nil {
		return nil, err
	}
	return clusters, nil
}

func (t *clusterTPR) Watch() error {
	logger.Infof("start watching %s tpr: %s", t.Name(), t.watchVersion)
	defer t.tracker.stop()

	eventCh, errCh := t.watch()

	go func() {
		timer := k8sutil.NewPanicTimer(
			time.Minute,
			fmt.Sprintf("unexpected long blocking (> 1 Minute) when handling %s event", t.Name()))

		for event := range eventCh {
			timer.Start()

			c := event.Object

			switch event.Type {
			case kwatch.Added:
				ns := c.Spec.Namespace
				if ns == "" {
					logger.Errorf("missing namespace attribute in rook spec")
					continue
				}

				newCluster := cluster.New(c.Spec, t.context.factory, t.context.clientset)
				t.tracker.add(ns, c.Metadata.ResourceVersion)
				t.clusters[ns] = newCluster

				logger.Infof("starting new cluster %s in namespace %s", c.Metadata.Name, ns)
				t.startCluster(newCluster)

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
func (t *clusterTPR) watch() (<-chan *clusterEvent, <-chan error) {
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

func (t *clusterTPR) watchOuterTPR(eventCh chan *clusterEvent, errCh chan error) error {
	resp, err := watchTPR(t.context, t.Name(), t.watchVersion)
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

		t.watchVersion = ev.Object.Metadata.ResourceVersion
		eventCh <- ev
	}
}

func (t *clusterTPR) checkStaleCache() (bool, error) {
	clusterList, err := t.getClusterList()
	if err == nil && !t.isClustersCacheStale(clusterList.Items) {
		t.watchVersion = clusterList.Metadata.ResourceVersion
		return false, nil
	}

	return true, err
}

func (t *clusterTPR) getRookClient(namespace string) (rookclient.RookRestClient, error) {
	t.Lock()
	defer t.Unlock()
	if c, ok := t.clusters[namespace]; ok {
		return c.GetRookClient()
	}

	return nil, fmt.Errorf("namespace %s not found", namespace)
}
