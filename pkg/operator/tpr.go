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

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const (
	tprGroup   = "rook.io"
	tprVersion = "v1alpha1"
)

type tprScheme interface {
	Name() string
	Description() string
}

type inclusterInitiator interface {
	Create(clusterMgr *clusterManager, name, namespace string) (tprManager, error)
}

type tprManager interface {
	Load() error
	Watch() error
	Manage()
}

func qualifiedName(tpr tprScheme) string {
	return fmt.Sprintf("%s.%s", tpr.Name(), tprGroup)
}

func createTPRs(context *clusterd.Context, tprs []tprScheme) error {
	for _, tpr := range tprs {
		if err := createTPR(context, tpr); err != nil {
			return fmt.Errorf("failed to init tpr %s. %+v", tpr.Name(), err)
		}
	}

	for _, tpr := range tprs {
		if err := waitForTPRInit(context, tpr); err != nil {
			return fmt.Errorf("failed to complete init %s. %+v", tpr.Name(), err)
		}
	}

	return nil
}

func createTPR(context *clusterd.Context, tpr tprScheme) error {
	logger.Infof("creating %s TPR", tpr.Name())
	r := &v1beta1.ThirdPartyResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: qualifiedName(tpr),
		},
		Versions: []v1beta1.APIVersion{
			{Name: tprVersion},
		},
		Description: tpr.Description(),
	}
	_, err := context.Clientset.ExtensionsV1beta1().ThirdPartyResources().Create(r)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s TPR. %+v", tpr.Name(), err)
		}
	}

	return nil
}

func waitForTPRInit(context *clusterd.Context, tpr tprScheme) error {
	restcli := context.Clientset.CoreV1().RESTClient()
	uri := tprURI(tpr.Name())
	return k8sutil.Retry(context, func() (bool, error) {
		_, err := restcli.Get().RequestURI(uri).DoRaw()
		if err != nil {
			logger.Infof("did not yet find tpr %s. %+v", tpr.Name(), err)
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func watchTPRNamespaced(context *clusterd.Context, name, namespace, resourceVersion string) (*http.Response, error) {
	uri := fmt.Sprintf("%s/%s?watch=true&resourceVersion=%s", context.MasterHost, tprURINamespaced(name, namespace), resourceVersion)
	logger.Debugf("watching tpr: %s", uri)
	return context.KubeHttpCli.Get(uri)
}

func watchTPR(context *clusterd.Context, name, resourceVersion string) (*http.Response, error) {
	uri := fmt.Sprintf("%s/%s?watch=true&resourceVersion=%s", context.MasterHost, tprURI(name), resourceVersion)
	logger.Debugf("watching tpr: %s", uri)
	return context.KubeHttpCli.Get(uri)
}

func tprURINamespaced(name, namespace string) string {
	//   /apis/rook.io/v1alpha1/namespaces/rook/pools
	return fmt.Sprintf("apis/%s/%s/namespaces/%s/%ss", tprGroup, tprVersion, namespace, name)
}

func tprURI(name string) string {
	// creates a uri that is for retrieving or watching a tpr in all namespaces. For example:
	//   /apis/rook.io/v1alpha1/clusters
	return fmt.Sprintf("apis/%s/%s/%ss", tprGroup, tprVersion, name)
}

func getRawListNamespaced(clientset kubernetes.Interface, name, namespace string) ([]byte, error) {
	restcli := clientset.CoreV1().RESTClient()
	uri := tprURINamespaced(name, namespace)
	logger.Debugf("getting tpr: %s", uri)
	return restcli.Get().RequestURI(uri).DoRaw()
}

func getRawList(clientset kubernetes.Interface, name string) ([]byte, error) {
	restcli := clientset.CoreV1().RESTClient()
	return restcli.Get().RequestURI(tprURI(name)).DoRaw()
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
