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
*/
package operator

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/client-go/1.5/kubernetes"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/operator/api"
	"github.com/rook/rook/pkg/operator/mon"
	"github.com/rook/rook/pkg/operator/osd"
	"github.com/rook/rook/pkg/operator/rgw"
)

type Operator struct {
	Namespace        string
	MasterHost       string
	containerVersion string
	clientset        *kubernetes.Clientset
	waitCluster      sync.WaitGroup
	factory          client.ConnectionFactory
}

func New(namespace string, factory client.ConnectionFactory, clientset *kubernetes.Clientset, containerVersion string) *Operator {
	return &Operator{
		Namespace:        namespace,
		factory:          factory,
		clientset:        clientset,
		containerVersion: containerVersion,
	}
}

func (o *Operator) Run() error {

	// Start the mon pods
	m := mon.New(o.Namespace, o.factory, o.containerVersion)
	cluster, err := m.Start(o.clientset)
	if err != nil {
		return fmt.Errorf("failed to start the mons. %+v", err)
	}

	a := api.New(o.Namespace, o.containerVersion)
	err = a.Start(o.clientset, cluster)
	if err != nil {
		return fmt.Errorf("failed to start the REST api. %+v", err)
	}

	// Start the OSDs
	osds := osd.New(o.Namespace, o.containerVersion)
	err = osds.Start(o.clientset, cluster)
	if err != nil {
		return fmt.Errorf("failed to start the osds. %+v", err)
	}

	// Start the object store
	r := rgw.New(o.Namespace, o.containerVersion, o.factory)
	err = r.Start(o.clientset, cluster)
	if err != nil {
		return fmt.Errorf("failed to start rgw. %+v", err)
	}

	logger.Infof("DONE!")
	<-time.After(1000000 * time.Second)

	return nil
}
