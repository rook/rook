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

	"github.com/rook/rook/pkg/operator/cluster"
	"github.com/rook/rook/pkg/operator/k8sutil"
	rookclient "github.com/rook/rook/pkg/rook/client"
	kwatch "k8s.io/client-go/pkg/watch"
)

type poolInitiator struct {
	context *context
}

type poolManager struct {
	name         string
	namespace    string
	watchVersion string
	context      *context
	rclient      rookclient.RookRestClient
}

func newPoolInitiator(context *context) *poolInitiator {
	return &poolInitiator{context: context}
}

func (p *poolInitiator) Create(clusterMgr *clusterManager, namespace string) (tprManager, error) {
	rclient, err := clusterMgr.getRookClient(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get api client for pool tpr in namespace %s. %+v", namespace, err)
	}
	return &poolManager{context: p.context, name: p.Name(), namespace: namespace, rclient: rclient}, nil
}

func (t *poolInitiator) Name() string {
	return "rookpool"
}

func (t *poolInitiator) Description() string {
	return "Managed Rook pools"
}

// Run the tpr manager until the caller signals with an EndWatch()
func (p *poolManager) Manage() {
	for {

		// load and initialize the pools
		if err := p.Load(); err != nil {
			logger.Errorf("cannot load %s pool tpr. %+v. retrying...", p.name, err)
		} else {
			// watch for added/updated/deleted pools
			if err := p.Watch(); err != nil {
				logger.Errorf("failed to watch %s pool tpr. %+v. retrying...", p.name, err)
			}
		}

		<-time.After(time.Second * time.Duration(p.context.retryDelay))
	}
}

func (t *poolManager) Load() error {
	// Check if pools have all been created
	logger.Info("finding existing pools...")
	poolList, err := t.getPoolList()
	if err != nil {
		return err
	}

	logger.Infof("found %d pools. ensuring they exist.", len(poolList.Items))
	for i := range poolList.Items {
		// ensure the pool exists
		p := poolList.Items[i]
		if err := p.Create(t.rclient); err != nil {
			logger.Warningf("failed to check that pool %s exists in namespace %s. %+v", p.Name, p.Namespace, err)
		}
	}

	t.watchVersion = poolList.Metadata.ResourceVersion
	return nil
}

func (t *poolManager) getPoolList() (*cluster.PoolList, error) {
	b, err := getRawList(t.context.clientset, t.name, t.namespace)
	if err != nil {
		return nil, err
	}

	pools := &cluster.PoolList{}
	if err := json.Unmarshal(b, pools); err != nil {
		return nil, err
	}
	return pools, nil
}

func (t *poolManager) Watch() error {
	logger.Infof("start watching pool tpr: %s", t.watchVersion)

	eventCh, errCh := t.watch()

	go func() {
		timer := k8sutil.NewPanicTimer(
			time.Minute,
			"unexpected long blocking (> 1 Minute) when handling pool event")

		for event := range eventCh {
			timer.Start()

			pool := event.Object

			switch event.Type {
			case kwatch.Added:
				if err := pool.Create(t.rclient); err != nil {
					logger.Errorf("failed to create pool %s. %+v", pool.Name, err)
					break
				}

			case kwatch.Modified:
				// if the pool is modified, allow the pool to be created if it wasn't already
				if err := pool.Create(t.rclient); err != nil {
					logger.Errorf("failed to create (modify) pool %s. %+v", pool.Name, err)
					break
				}

			case kwatch.Deleted:
				err := pool.Delete(t.rclient)
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
func (t *poolManager) watch() (<-chan *poolEvent, <-chan error) {
	eventCh := make(chan *poolEvent)
	// On unexpected error case, the operator should exit
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)

		for {
			err := t.watchOuterTPR(eventCh, errCh)
			if err != nil {
				errCh <- fmt.Errorf("failed to watch pool tpr. %+v", err)
				return
			}
		}
	}()

	return eventCh, errCh
}

func (t *poolManager) watchOuterTPR(eventCh chan *poolEvent, errCh chan error) error {
	resp, err := watchTPR(t.context, t.name, t.namespace, t.watchVersion)
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
		done, err := handlePollEventResult(st, err, func() (bool, error) { return false, nil }, errCh)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		logger.Debugf("rook pool event: %+v", ev)

		t.watchVersion = ev.Object.ResourceVersion
		eventCh <- ev
	}
}
