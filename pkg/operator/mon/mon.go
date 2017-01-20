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
package mon

import (
	"fmt"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
)

const (
	MonPort        = 6790
	monApp         = "cephmon"
	monNodeAttr    = "mon_node"
	monClusterAttr = "mon_cluster"
	tprName        = "mon.rook.io"
)

type Cluster struct {
	Namespace    string
	Keyring      string
	Version      string
	MasterHost   string
	Size         int
	Paused       bool
	NodeSelector map[string]string
	AntiAffinity bool
	Port         int32
	factory      client.ConnectionFactory
}

type MonConfig struct {
	Name string
	Port int32
	Info *mon.ClusterInfo
}

func New(namespace string, factory client.ConnectionFactory) *Cluster {
	return &Cluster{
		Namespace:    namespace,
		Version:      k8sutil.RookContainerVersion,
		Size:         3,
		factory:      factory,
		AntiAffinity: true,
	}
}

func (c *Cluster) Start(clientset *kubernetes.Clientset) error {
	logger.Infof("start running one mon")

	clusterInfo, err := c.initClusterInfo()
	if err != nil {
		return fmt.Errorf("failed to initialize ceph cluster info. %+v", err)
	}

	// schedule the mons on different nodes if we have enough nodes to be unique
	err = c.setAntiAffinity(clientset)
	if err != nil {
		return fmt.Errorf("failed to set antiaffinity. %+v", err)
	}

	running, pending, err := c.pollPods(clientset, clusterInfo)
	if err != nil {
		return fmt.Errorf("failed to get mon pods. %+v", err)
	}
	logger.Infof("Running pods: %+v", flattenMonEndpoints(podsToMonEndpoints(running)))
	logger.Infof("%d pending pods", pending)

	if len(running) > 0 {
		logger.Infof("pods are already running")
		return nil
	}

	started := 0
	alreadyRunning := 0
	for i := 0; i < c.Size; i++ {
		mon := &MonConfig{Name: fmt.Sprintf("mon%d", i), Info: clusterInfo, Port: int32(MonPort)}
		monPod := c.makeMonPod(mon)
		logger.Debugf("Starting pod: %+v", monPod)
		_, err := clientset.Pods(c.Namespace).Create(monPod)
		if err != nil {
			if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
				return fmt.Errorf("failed to create mon pod %s. %+v", c.Namespace, err)
			}
			alreadyRunning++
			logger.Infof("mon pod %s already exists", monPod.Name)
		} else {
			started++
		}
	}

	logger.Infof("started %d/%d mons (%d already running)", (started + alreadyRunning), c.Size, alreadyRunning)
	return nil
}

// Retrieve the ceph cluster info if it already exists.
// If a new cluster create new keys.
func (c *Cluster) initClusterInfo() (*mon.ClusterInfo, error) {
	return mon.CreateClusterInfo(c.factory, "")
}

func (c *Cluster) setAntiAffinity(clientset *kubernetes.Clientset) error {
	nodeOptions := api.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	nodes, err := clientset.Nodes().List(nodeOptions)
	if err != nil {
		return fmt.Errorf("failed to get nodes in cluster. %+v", err)
	}

	logger.Infof("there are %d nodes available for %d monitors", len(nodes.Items), c.Size)
	c.AntiAffinity = len(nodes.Items) >= c.Size
	return nil
}
