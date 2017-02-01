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
	"time"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
)

const (
	monApp         = "cephmon"
	monNodeAttr    = "mon_node"
	monClusterAttr = "mon_cluster"
	tprName        = "mon.rook.io"
)

type Cluster struct {
	Namespace    string
	Keyring      string
	ClusterName  string
	Version      string
	MasterHost   string
	Size         int
	Paused       bool
	AntiAffinity bool
	Port         int32
	factory      client.ConnectionFactory
}

type MonConfig struct {
	Name string
	Port int32
	Info *mon.ClusterInfo
}

func New(namespace string, factory client.ConnectionFactory, version string) *Cluster {
	return &Cluster{
		Namespace:    namespace,
		Version:      version,
		Size:         3,
		factory:      factory,
		AntiAffinity: true,
	}
}

func (c *Cluster) Start(clientset *kubernetes.Clientset) (*mon.ClusterInfo, error) {
	logger.Infof("start running mons")

	clusterInfo, err := c.initClusterInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ceph cluster info. %+v", err)
	}
	mons := []*MonConfig{}
	for i := 0; i < c.Size; i++ {
		mons = append(mons, &MonConfig{Name: fmt.Sprintf("mon%d", i), Info: clusterInfo, Port: int32(mon.Port)})
	}

	err = c.startPods(clientset, clusterInfo, mons)
	if err != nil {
		return nil, fmt.Errorf("failed to start mon pods. %+v", err)
	}

	return clusterInfo, nil
}

// Retrieve the ceph cluster info if it already exists.
// If a new cluster create new keys.
func (c *Cluster) initClusterInfo() (*mon.ClusterInfo, error) {
	return mon.CreateClusterInfo(c.factory, "")
}

func (c *Cluster) startPods(clientset *kubernetes.Clientset, clusterInfo *mon.ClusterInfo, mons []*MonConfig) error {
	// schedule the mons on different nodes if we have enough nodes to be unique
	antiAffinity, err := c.getAntiAffinity(clientset)
	if err != nil {
		return fmt.Errorf("failed to get antiaffinity. %+v", err)
	}

	running, pending, err := c.pollPods(clientset, clusterInfo.Name)
	if err != nil {
		return fmt.Errorf("failed to get mon pods. %+v", err)
	}
	logger.Infof("%d running, %d pending pods", len(running), len(pending))

	if len(running) == c.Size {
		logger.Infof("pods are already running")
		return nil
	}

	clusterInfo.Monitors = map[string]*mon.CephMonitorConfig{}
	for _, m := range running {
		clusterInfo.Monitors[m.Name] = mon.ToCephMon(m.Name, m.Status.PodIP)
	}

	started := 0
	alreadyRunning := 0
	for _, m := range mons {
		monPod := c.makeMonPod(m, clusterInfo, antiAffinity)
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

		podIP, err := c.waitForPodToStart(clientset, monPod)
		if err != nil {
			return fmt.Errorf("failed to start pod %s. %+v", monPod.Name, err)
		}
		clusterInfo.Monitors[m.Name] = mon.ToCephMon(m.Name, podIP)
	}

	logger.Infof("started %d/%d mons (%d already running)", (started + alreadyRunning), c.Size, alreadyRunning)
	return nil
}

func (c *Cluster) waitForPodToStart(clientset *kubernetes.Clientset, pod *v1.Pod) (string, error) {

	// Poll the status of the pods to see if they are ready
	// FIX: Get status instead of just waiting
	for i := 0; i < 15; i++ {
		// wait and try again
		delay := 8
		logger.Infof("waiting %ds for pod %s to start. status=%v", delay, pod.Name, pod.Status.Phase)
		<-time.After(time.Duration(delay) * time.Second)

		pod, err := clientset.Core().Pods(c.Namespace).Get(pod.Name)
		if err != nil {
			return "", fmt.Errorf("failed to get mon pod %s. %+v", pod.Name, err)
		}

		if pod.Status.Phase == v1.PodRunning {
			logger.Infof("pod %s started", pod.Name)
			return pod.Status.PodIP, nil
		}
	}

	return "", fmt.Errorf("timed out waiting for pod %s to start", pod.Name)
}

// detect whether we have a big enough cluster to run services on different nodes.
// the anti-affinity will prevent pods of the same type of running on the same node.
func (c *Cluster) getAntiAffinity(clientset *kubernetes.Clientset) (bool, error) {
	nodeOptions := api.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	nodes, err := clientset.Nodes().List(nodeOptions)
	if err != nil {
		return false, fmt.Errorf("failed to get nodes in cluster. %+v", err)
	}

	logger.Infof("there are %d nodes available for %d monitors", len(nodes.Items), c.Size)
	return len(nodes.Items) >= c.Size, nil
}

func (c *Cluster) GetMonPodsRunning(clientset *kubernetes.Clientset, clusterName string) (int, int, error) {
	running, pending, err := c.pollPods(clientset, clusterName)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get mon pods. %+v", err)
	}
	return len(running), len(pending), nil
}
