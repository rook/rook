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
	"fmt"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/cluster"
	"github.com/rook/rook/pkg/operator/kit"
	rookclient "github.com/rook/rook/pkg/rook/client"
	kwatch "k8s.io/apimachinery/pkg/watch"
)

type poolInitiator struct {
	context *clusterd.Context
}

type poolManager struct {
	namespace    string
	watchVersion string
	context      *clusterd.Context
	rclient      rookclient.RookRestClient
}

type poolEvent struct {
	Type   kwatch.EventType
	Object *cluster.Pool
}

func newPoolInitiator(context *clusterd.Context) *poolInitiator {
	return &poolInitiator{context: context}
}

func (p *poolInitiator) Create(clusterMgr *clusterManager, namespace string) (resourceManager, error) {
	rclient, err := clusterMgr.getRookClient(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get api client for pool tpr for cluster in namespace %s. %+v", namespace, err)
	}
	return &poolManager{context: p.context, namespace: namespace, rclient: rclient}, nil
}

func (p *poolInitiator) Resource() kit.CustomResource {
	return cluster.PoolResource
}

// Run the tpr manager until the caller signals with an EndWatch()
func (p *poolManager) Manage() {
	for {

		// load and initialize the pools
		watchVersion, err := p.Load()
		if err != nil {
			logger.Errorf("cannot load %s pool tpr. %+v. retrying...", p.namespace, err)
		} else {
			// watch for added/updated/deleted pools
			watcher := kit.NewWatcher(p.context.KubeContext, cluster.PoolResource, p.namespace, watchVersion, p.handlePoolEvent, nil)
			if err := watcher.Watch(); err != nil {
				logger.Errorf("failed to watch %s pool tpr. %+v. retrying...", p.namespace, err)
			}
		}

		<-time.After(time.Second * time.Duration(p.context.RetryDelay))
	}
}

func (p *poolManager) handlePoolEvent(event *kit.RawEvent) error {
	pool := &poolEvent{
		Type:   event.Type,
		Object: &cluster.Pool{},
	}
	err := json.Unmarshal(event.Object, pool.Object)
	if err != nil {
		return fmt.Errorf("fail to unmarshal Pool from data (%s): %v", pool.Object, err)
	}

	switch event.Type {
	case kwatch.Added:
		if err := pool.Object.Create(p.rclient); err != nil {
			logger.Errorf("failed to create pool %s. %+v", pool.Object.Name, err)
			break
		}

	case kwatch.Modified:
		// if the pool is modified, allow the pool to be created if it wasn't already
		if err := pool.Object.Create(p.rclient); err != nil {
			logger.Errorf("failed to create (modify) pool %s. %+v", pool.Object.Name, err)
			break
		}

	case kwatch.Deleted:
		err := pool.Object.Delete(p.rclient)
		if err != nil {
			logger.Errorf("failed to delete pool %s. %+v", pool.Object.Name, err)
			break
		}
	}
	return nil
}

func (p *poolManager) Load() (string, error) {
	// Check if pools have all been created
	logger.Info("finding existing pools...")
	poolList, err := p.getPoolList()
	if err != nil {
		return "", err
	}

	logger.Infof("found %d pools. ensuring they exist.", len(poolList.Items))
	for i := range poolList.Items {
		// ensure the pool exists
		item := poolList.Items[i]
		logger.Infof("checking pool %s in namespace %s", item.Name, item.Namespace)
		if err := item.Create(p.rclient); err != nil {
			logger.Warningf("failed to check that pool %s exists in namespace %s. %+v", item.Name, item.Namespace, err)
		}
	}

	return poolList.Metadata.ResourceVersion, nil
}

func (p *poolManager) getPoolList() (*cluster.PoolList, error) {
	b, err := kit.GetRawListNamespaced(p.context.Clientset, cluster.PoolResource, p.namespace)
	if err != nil {
		return nil, err
	}

	pools := &cluster.PoolList{}
	if err := json.Unmarshal(b, pools); err != nil {
		return nil, err
	}
	return pools, nil
}
