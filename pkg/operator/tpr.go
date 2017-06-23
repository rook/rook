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

Some of the code was modified from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/
package operator

import (
	"fmt"
	"io"
	"net/http"

	"github.com/rook/rook/pkg/operator/k8sutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type inclusterInitiator interface {
	Create(clusterMgr *clusterManager, namespace string) (k8sutil.TPRManager, error)
	Scheme() k8sutil.TPRScheme
}

func handlePollEventResult(status *metav1.Status, errIn error, checkStaleCache func() (bool, error), errCh chan error) (done bool, err error) {
	if errIn != nil {
		if errIn == io.EOF { // apiserver will close stream periodically
			logger.Debug("apiserver closed stream")
			done = true
			return
		}

		err = errIn
		errCh <- fmt.Errorf("received invalid event from API server: %v", err)
		return
	}

	if status != nil {

		if status.Code == http.StatusGone {
			// event history is outdated.
			// if nothing has changed, we can go back to watch again.
			var stale bool
			stale, err = checkStaleCache()
			if err == nil && !stale {
				done = true
				return
			}

			// if anything has changed (or error on relist), we have to rebuild the state.
			// go to recovery path
			err = ErrVersionOutdated
			errCh <- ErrVersionOutdated
			return
		}

		logger.Errorf("unexpected status response from API server: %v", status.Message)
		done = true
		return
	}
	return
}
