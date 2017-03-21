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
	"io"
	"net/http"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"

	"k8s.io/client-go/rest"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/operator/cluster"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/runtime"
	"k8s.io/client-go/pkg/runtime/serializer"
	kwatch "k8s.io/client-go/pkg/watch"
)

const (
	initRetryDelay = 10 * time.Second
)

var (
	ErrVersionOutdated = errors.New("requested version is outdated in apiserver")
)

type Operator struct {
	Namespace    string
	MasterHost   string
	devicesInUse bool
	retryDelay   int
	clientset    kubernetes.Interface
	waitCluster  sync.WaitGroup
	factory      client.ConnectionFactory
	stopChMap    map[string]chan struct{}
	clusters     map[string]*cluster.Cluster
	kubeHttpCli  *http.Client
	// Kubernetes resource version of the clusters
	clusterRVs map[string]string
}

func New(host, namespace string, factory client.ConnectionFactory, clientset kubernetes.Interface) *Operator {
	return &Operator{
		Namespace:  namespace,
		MasterHost: host,
		factory:    factory,
		clientset:  clientset,
		clusters:   make(map[string]*cluster.Cluster),
		clusterRVs: make(map[string]string),
		stopChMap:  map[string]chan struct{}{},
		retryDelay: 3,
	}
}

func (o *Operator) Run() error {
	var watchVersion string
	var err error
	for {
		watchVersion, err = o.initResources()
		if err == nil {
			break
		}
		logger.Errorf("failed to init resources. %+v. retrying...", err)
		<-time.After(initRetryDelay)
	}

	// watch for changes to the rook clusters
	return o.watchTPR(watchVersion)
}

func (o *Operator) initResources() (string, error) {
	httpCli, err := newHttpClient()
	if err != nil {
		return "", fmt.Errorf("failed to get tpr client. %+v", err)
	}
	o.kubeHttpCli = httpCli.Client

	err = o.createTPR()
	if err != nil {
		return "", fmt.Errorf("failed to create TPR. %+v", err)
	}
	err = o.waitForTPRInit(o.clientset.CoreV1().RESTClient(), 30, o.Namespace)
	if err != nil {
		return "", fmt.Errorf("failed to wait for TPR. %+v", err)
	}

	// Check if there is an existing cluster to recover
	watchVersion, err := o.findAllClusters()
	if err != nil {
		return "", fmt.Errorf("failed to find clusters. %+v", err)
	}
	return watchVersion, nil
}

func (o *Operator) watchTPR(watchVersion string) error {
	logger.Infof("start watching rook tpr: %s", watchVersion)
	defer func() {
		for _, stop := range o.stopChMap {
			close(stop)
		}
		o.waitCluster.Wait()
	}()

	eventCh, errCh := o.watch(watchVersion)

	go func() {
		pt := k8sutil.NewPanicTimer(time.Minute, "unexpected long blocking (> 1 Minute) when handling cluster event")

		for event := range eventCh {
			pt.Start()

			c := event.Object

			switch event.Type {
			case kwatch.Added:
				ns := c.Spec.Namespace
				if ns == "" {
					logger.Errorf("missing namespace attribute in rook spec")
					continue
				}

				newCluster := cluster.New(c.Spec, o.factory, o.clientset)
				stopCh := make(chan struct{})
				o.stopChMap[ns] = stopCh
				o.clusters[ns] = newCluster
				o.clusterRVs[ns] = c.Metadata.ResourceVersion

				logger.Infof("starting new cluster %s in namespace %s", c.Metadata.Name, ns)
				o.startCluster(newCluster)

			case kwatch.Modified:
				logger.Infof("modifying a cluster not implemented")

			case kwatch.Deleted:
				logger.Infof("deleting a cluster not implemented")
			}

			pt.Stop()
		}
	}()
	return <-errCh

}

func (o *Operator) startCluster(c *cluster.Cluster) {
	if o.devicesInUse && c.Spec.UseAllDevices {
		logger.Warningf("devices in more than one namespace not supported. ignoring devices in namespace %s", c.Spec.Namespace)
		c.Spec.UseAllDevices = false
	}

	if c.Spec.UseAllDevices {
		o.devicesInUse = true
	}

	go func() {
		err := c.CreateInstance()
		if err != nil {
			logger.Errorf("failed to create cluster in namespace %s. %+v", c.Spec.Namespace, err)
			return
		}
		c.Monitor(o.stopChMap[c.Spec.Namespace])
	}()
}

func (o *Operator) findAllClusters() (string, error) {
	logger.Info("finding existing clusters...")
	clusterList, err := getClusterList(o.clientset.CoreV1().RESTClient(), o.Namespace)
	if err != nil {
		return "", err
	}
	logger.Infof("found %d clusters", len(clusterList.Items))
	for i := range clusterList.Items {
		c := clusterList.Items[i]

		stopCh := make(chan struct{})
		ns := c.Spec.Namespace
		existingCluster := cluster.New(c.Spec, o.factory, o.clientset)
		o.stopChMap[ns] = stopCh
		o.clusters[ns] = existingCluster
		o.clusterRVs[ns] = c.Metadata.ResourceVersion

		logger.Infof("resuming cluster %s in namespace %s", c.Metadata.Name, ns)
		o.startCluster(existingCluster)
	}

	return clusterList.Metadata.ResourceVersion, nil
}

// watch creates a go routine, and watches the cluster.rook kind resources from
// the given watch version. It emits events on the resources through the returned
// event chan. Errors will be reported through the returned error chan. The go routine
// exits on any error.
func (o *Operator) watch(watchVersion string) (<-chan *Event, <-chan error) {
	eventCh := make(chan *Event)
	// On unexpected error case, the operator should exit
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)

		for {
			resp, err := watchClusters(o.MasterHost, o.Namespace, o.kubeHttpCli, watchVersion)
			if err != nil {
				errCh <- err
				return
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				errCh <- errors.New("invalid status code: " + resp.Status)
				return
			}

			decoder := json.NewDecoder(resp.Body)
			for {
				ev, st, err := pollEvent(decoder)
				if err != nil {
					if err == io.EOF { // apiserver will close stream periodically
						logger.Debug("apiserver closed stream")
						break
					}

					logger.Errorf("received invalid event from API server: %v", err)
					errCh <- err
					return
				}

				if st != nil {
					resp.Body.Close()

					if st.Code == http.StatusGone {
						// event history is outdated.
						// if nothing has changed, we can go back to watch again.
						clusterList, err := getClusterList(o.clientset.CoreV1().RESTClient(), o.Namespace)
						if err == nil && !o.isClustersCacheStale(clusterList.Items) {
							watchVersion = clusterList.Metadata.ResourceVersion
							break
						}

						// if anything has changed (or error on relist), we have to rebuild the state.
						// go to recovery path
						errCh <- ErrVersionOutdated
						return
					}

					logger.Errorf("unexpected status response from API server: %v", st.Message)
					break
				}

				logger.Debugf("rook cluster event: %+v", ev)

				watchVersion = ev.Object.Metadata.ResourceVersion
				eventCh <- ev
			}

			resp.Body.Close()
		}
	}()

	return eventCh, errCh
}

func (o *Operator) isClustersCacheStale(currentClusters []cluster.Cluster) bool {
	if len(o.clusterRVs) != len(currentClusters) {
		return true
	}

	for _, cc := range currentClusters {
		rv, ok := o.clusterRVs[cc.Metadata.Name]
		if !ok || rv != cc.Metadata.ResourceVersion {
			return true
		}
	}

	return false
}

func watchClusters(host, ns string, httpClient *http.Client, resourceVersion string) (*http.Response, error) {
	return httpClient.Get(fmt.Sprintf("%s/apis/%s/%s/namespaces/%s/clusters?watch=true&resourceVersion=%s",
		host, tprGroup, tprVersion, ns, resourceVersion))
}

func getClusterList(restcli rest.Interface, ns string) (*cluster.ClusterList, error) {
	b, err := restcli.Get().RequestURI(listClustersURI(ns)).DoRaw()
	if err != nil {
		return nil, err
	}

	clusters := &cluster.ClusterList{}
	if err := json.Unmarshal(b, clusters); err != nil {
		return nil, err
	}
	return clusters, nil
}

func listClustersURI(ns string) string {
	return fmt.Sprintf("/apis/%s/%s/namespaces/%s/clusters", tprGroup, tprVersion, ns)
}

func newHttpClient() (*rest.RESTClient, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	config.GroupVersion = &unversioned.GroupVersion{
		Group:   tprGroup,
		Version: tprVersion,
	}
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}

	restcli, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, err
	}
	return restcli, nil
}
