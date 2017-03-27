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

	kwatch "k8s.io/client-go/pkg/watch"

	"github.com/rook/rook/pkg/operator/cluster"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

type poolTPR struct {
	context      *context
	watchVersion string
	cluster      *clusterTPR
}

func newPoolTPR(context *context, cluster *clusterTPR) *poolTPR {
	return &poolTPR{context: context, cluster: cluster}
}

func (t *poolTPR) Name() string {
	return "rookpool"
}

func (t *poolTPR) Description() string {
	return "Managed Rook pools"
}

func (t *poolTPR) Load() error {
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
		rclient, err := t.cluster.getRookClient(p.Namespace)
		if err != nil {
			return fmt.Errorf("failed to get rook client for namespace %s. %+v", p.Namespace, err)
		}
		if err := p.Create(rclient); err != nil {
			logger.Warningf("failed to check that pool %s exists in namespace %s. %+v", p.PoolSpec.Name, p.PoolSpec.Namespace, err)
		}
	}

	t.watchVersion = poolList.Metadata.ResourceVersion
	return nil
}

func (t *poolTPR) getPoolList() (*cluster.PoolList, error) {
	b, err := getRawList(t.context, t)
	if err != nil {
		return nil, err
	}

	pools := &cluster.PoolList{}
	if err := json.Unmarshal(b, pools); err != nil {
		return nil, err
	}
	return pools, nil
}

func (t *poolTPR) Watch() error {
	logger.Infof("start watching %s tpr: %s", t.Name(), t.watchVersion)

	eventCh, errCh := t.watch()

	go func() {
		timer := k8sutil.NewPanicTimer(
			time.Minute,
			fmt.Sprintf("unexpected long blocking (> 1 Minute) when handling %s event", t.Name()))

		for event := range eventCh {
			timer.Start()

			pool := event.Object
			rclient, err := t.cluster.getRookClient(pool.PoolSpec.Namespace)
			if err != nil {
				logger.Errorf("failed %v pool %s. %+v", event.Type, pool.Name, err)
				break
			}

			switch event.Type {
			case kwatch.Added:
				p := cluster.NewPool(pool.PoolSpec)
				if err := p.Create(rclient); err != nil {
					logger.Errorf("failed to create pool %s. %+v", pool.PoolSpec.Name, err)
					break
				}

			case kwatch.Modified:
				// if the pool is modified, allow the pool to be created if it wasn't already
				p := cluster.NewPool(pool.PoolSpec)
				if err := p.Create(rclient); err != nil {
					logger.Errorf("failed to create (modify) pool %s. %+v", pool.PoolSpec.Name, err)
					break
				}

			case kwatch.Deleted:
				oldPool := cluster.NewPool(pool.PoolSpec)
				err := oldPool.Delete(rclient)
				if err != nil {
					logger.Errorf("failed to delete pool %s. %+v", pool.PoolSpec.Name, err)
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
func (t *poolTPR) watch() (<-chan *poolEvent, <-chan error) {
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

func (t *poolTPR) watchOuterTPR(eventCh chan *poolEvent, errCh chan error) error {
	resp, err := watchTPR(t.context, t.Name(), t.watchVersion)
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

		t.watchVersion = ev.Object.Metadata.ResourceVersion
		eventCh <- ev
	}
}
