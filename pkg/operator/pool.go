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

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/cluster"
	"github.com/rook/rook/pkg/operator/k8sutil"
	rookclient "github.com/rook/rook/pkg/rook/client"
	kwatch "k8s.io/apimachinery/pkg/watch"
)

const (
	poolTprName = "pool"
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

func newPoolInitiator(context *clusterd.Context) *poolInitiator {
	return &poolInitiator{context: context}
}

func (p *poolInitiator) Create(clusterMgr *clusterManager, clusterName, namespace string) (tprManager, error) {
	rclient, err := clusterMgr.getRookClient(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get api client for pool tpr for cluster %s in namespace %s. %+v", clusterName, namespace, err)
	}
	return &poolManager{context: p.context, namespace: namespace, rclient: rclient}, nil
}

func (p *poolInitiator) Name() string {
	return poolTprName
}

func (p *poolInitiator) Description() string {
	return "Managed Rook pools"
}

// Run the tpr manager until the caller signals with an EndWatch()
func (p *poolManager) Manage() {
	for {

		// load and initialize the pools
		if err := p.Load(); err != nil {
			logger.Errorf("cannot load %s pool tpr. %+v. retrying...", p.namespace, err)
		} else {
			// watch for added/updated/deleted pools
			if err := p.Watch(); err != nil {
				logger.Errorf("failed to watch %s pool tpr. %+v. retrying...", p.namespace, err)
			}
		}

		<-time.After(time.Second * time.Duration(p.context.RetryDelay))
	}
}

func (p *poolManager) Load() error {
	// Check if pools have all been created
	logger.Info("finding existing pools...")
	poolList, err := p.getPoolList()
	if err != nil {
		return err
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

	p.watchVersion = poolList.Metadata.ResourceVersion
	return nil
}

func (p *poolManager) getPoolList() (*cluster.PoolList, error) {
	b, err := getRawListNamespaced(p.context.Clientset, poolTprName, p.namespace)
	if err != nil {
		return nil, err
	}

	pools := &cluster.PoolList{}
	if err := json.Unmarshal(b, pools); err != nil {
		return nil, err
	}
	return pools, nil
}

func (p *poolManager) Watch() error {
	logger.Infof("start watching pool tpr in namespace %s: %s", p.namespace, p.watchVersion)

	eventCh, errCh := p.watch()

	go func() {
		timer := k8sutil.NewPanicTimer(
			time.Minute,
			"unexpected long blocking (> 1 Minute) when handling pool event")

		for event := range eventCh {
			timer.Start()

			pool := event.Object

			switch event.Type {
			case kwatch.Added:
				if err := pool.Create(p.rclient); err != nil {
					logger.Errorf("failed to create pool %s. %+v", pool.Name, err)
					break
				}

			case kwatch.Modified:
				// if the pool is modified, allow the pool to be created if it wasn't already
				if err := pool.Create(p.rclient); err != nil {
					logger.Errorf("failed to create (modify) pool %s. %+v", pool.Name, err)
					break
				}

			case kwatch.Deleted:
				err := pool.Delete(p.rclient)
				if err != nil {
					logger.Errorf("failed to delete pool %s. %+v", pool.Name, err)
					break
				}
			}

			timer.Stop()
		}
	}()
	return <-errCh

}

// watch creates a go routine, and watches the pool.rook.io kind resources from
// the given watch version. It emits events on the resources through the returned
// event chan. Errors will be reported through the returned error chan. The go routine
// exits on any error.
func (p *poolManager) watch() (<-chan *poolEvent, <-chan error) {
	eventCh := make(chan *poolEvent)
	// On unexpected error case, the operator should exit
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)

		for {
			err := p.watchOuterTPR(eventCh, errCh)
			if err != nil {
				errCh <- fmt.Errorf("failed to watch pool tpr. %+v", err)
				return
			}
		}
	}()

	return eventCh, errCh
}

func (p *poolManager) watchOuterTPR(eventCh chan *poolEvent, errCh chan error) error {
	resp, err := watchTPRNamespaced(p.context, poolTprName, p.namespace, p.watchVersion)
	if err != nil {
		errCh <- err
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("invalid status code: " + resp.Status)
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		ev, st, err := pollPoolEvent(decoder)
		done, err := handlePollEventResult(st, err, func() (bool, error) {
			// there is no cache for the pool tpr, so we return true to indicate to always reset the watch on error
			return true, nil
		}, errCh)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		logger.Debugf("rook pool event: %+v", ev)

		p.watchVersion = ev.Object.ResourceVersion
		eventCh <- ev
	}
}
