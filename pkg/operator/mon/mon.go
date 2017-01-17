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

	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/client-go/1.5/kubernetes"
)

const (
	dataDir     = "/var/lib/rook/data"
	MonPort     = 6790
	monApp      = "cephmon"
	appAttr     = "app"
	clusterAttr = "rook_cluster"
	monNodeAttr = "mon_node"
	versionAttr = "mon_version"
	tprName     = "mon.rook.io"
)

type Cluster struct {
	Namespace    string
	Keyring      string
	ClusterName  string
	DataDir      string
	Version      string
	MonConfig    []*MonConfig
	MasterHost   string
	Size         int
	Paused       bool
	NodeSelector map[string]string
	AntiAffinity bool
	Port         int32
}

func New(namespace string) *Cluster {
	return &Cluster{
		Namespace:   namespace,
		ClusterName: defaultClusterName,
		DataDir:     dataDir,
		Version:     "dev-2017-01-06-e",
		Size:        3,
		Port:        MonPort,
	}
}

func (c *Cluster) Start(clientset *kubernetes.Clientset) error {
	logger.Infof("start running one mon")

	started := 0
	alreadyRunning := 0
	for i := 0; i < c.Size; i++ {
		mon := &MonConfig{Name: fmt.Sprintf("mon%d", i), Port: MonPort, InitialMons: []string{}}
		monPod := c.makeMonPod(mon)
		_, err := clientset.Pods(c.Namespace).Create(monPod)
		if err != nil {
			if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
				return fmt.Errorf("failed to create namespace %s. %+v", c.Namespace, err)
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
