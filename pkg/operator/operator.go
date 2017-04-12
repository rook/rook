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
	"errors"
	"fmt"
	"net/http"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/rook/rook/pkg/cephmgr/client"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/pkg/api"
)

const (
	initRetryDelay = 10 * time.Second
)

var (
	ErrVersionOutdated = errors.New("requested version is outdated in apiserver")
)

type context struct {
	clientset   kubernetes.Interface
	retryDelay  int
	maxRetries  int
	masterHost  string
	kubeHttpCli *http.Client
	factory     client.ConnectionFactory
}

type Operator struct {
	context    *context
	tprSchemes []tprScheme
	// The TPR that is global to the kubernetes cluster.
	// The cluster TPR is global because you create multiple clusers in k8s
	clusterMgr *clusterManager
}

func New(host string, factory client.ConnectionFactory, clientset kubernetes.Interface) *Operator {
	context := &context{
		masterHost: host,
		factory:    factory,
		clientset:  clientset,
		retryDelay: 3,
		maxRetries: 30,
	}
	poolInitiator := newPoolInitiator(context)
	clusterMgr := newClusterManager(context, []inclusterInitiator{poolInitiator})
	schemes := []tprScheme{clusterMgr, poolInitiator}
	return &Operator{
		context:    context,
		clusterMgr: clusterMgr,
		tprSchemes: schemes,
	}
}

func (o *Operator) Run() error {

	for {
		err := o.initResources()
		if err == nil {
			break
		}
		logger.Errorf("failed to init resources. %+v. retrying...", err)
		<-time.After(initRetryDelay)
	}

	// watch for changes to the rook clusters
	o.clusterMgr.Manage()
	return nil
}

func (o *Operator) initResources() error {
	httpCli, err := newHttpClient()
	if err != nil {
		return fmt.Errorf("failed to get tpr client. %+v", err)
	}
	o.context.kubeHttpCli = httpCli.Client

	err = createTPRs(o.context, o.tprSchemes)
	if err != nil {
		return fmt.Errorf("failed to create TPR. %+v", err)
	}

	return nil
}

func newHttpClient() (*rest.RESTClient, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	config.GroupVersion = &schema.GroupVersion{
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
