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

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api/v1"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/mon"
)

type Operator struct {
	Namespace   string
	MasterHost  string
	clientset   *kubernetes.Clientset
	waitCluster sync.WaitGroup
	factory     client.ConnectionFactory
}

func New(namespace string, factory client.ConnectionFactory, clientset *kubernetes.Clientset) *Operator {
	return &Operator{
		Namespace: namespace,
		factory:   factory,
		clientset: clientset,
	}
}

func (o *Operator) Run() error {

	// Create the namespace
	logger.Infof("Creating namespace %s", o.Namespace)
	ns := &v1.Namespace{}
	ns.Name = o.Namespace
	_, err := o.clientset.Namespaces().Create(ns)
	if err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return fmt.Errorf("failed to create namespace %s. %+v", o.Namespace, err)
		}
		logger.Infof("namespace %s already exists", o.Namespace)
	}

	// Start the mon pods
	m := mon.New(o.Namespace)
	err = m.Start(o.clientset)
	if err != nil {
		return fmt.Errorf("failed to start the mons. %+v", err)
	}

	// Start the OSDs

	return nil
}
