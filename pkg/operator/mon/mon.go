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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	appName           = "mon"
	monNodeAttr       = "mon_node"
	monClusterAttr    = "mon_cluster"
	tprName           = "mon.rook.io"
	fsidSecretName    = "fsid"
	monSecretName     = "mon-secret"
	adminSecretName   = "admin-secret"
	clusterSecretName = "cluster-name"
)

type Cluster struct {
	Namespace       string
	Keyring         string
	ClusterName     string
	Version         string
	MasterHost      string
	Size            int
	Paused          bool
	AntiAffinity    bool
	Port            int32
	factory         client.ConnectionFactory
	retryDelay      int
	dataDirHostPath string
}

type MonConfig struct {
	Name string
	Port int32
}

func New(namespace string, factory client.ConnectionFactory, dataDirHostPath, version string) *Cluster {
	return &Cluster{
		Namespace:       namespace,
		Version:         version,
		Size:            3,
		factory:         factory,
		dataDirHostPath: dataDirHostPath,
		AntiAffinity:    true,
		retryDelay:      6,
	}
}

func (c *Cluster) Start(clientset kubernetes.Interface) (*mon.ClusterInfo, error) {
	logger.Infof("start running mons")

	clusterInfo, err := c.initClusterInfo(clientset)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ceph cluster info. %+v", err)
	}
	mons := []*MonConfig{}
	for i := 0; i < c.Size; i++ {
		mons = append(mons, &MonConfig{Name: fmt.Sprintf("mon%d", i), Port: int32(mon.Port)})
	}

	err = c.startPods(clientset, clusterInfo, mons)
	if err != nil {
		return nil, fmt.Errorf("failed to start mon pods. %+v", err)
	}

	return clusterInfo, nil
}

// Retrieve the ceph cluster info if it already exists.
// If a new cluster create new keys.
func (c *Cluster) initClusterInfo(clientset kubernetes.Interface) (*mon.ClusterInfo, error) {
	secrets, err := clientset.CoreV1().Secrets(c.Namespace).Get(appName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get mon secrets. %+v", err)
		}

		return c.createMonSecretsAndSave(clientset)
	}

	info := &mon.ClusterInfo{
		Name:          string(secrets.Data[clusterSecretName]),
		FSID:          string(secrets.Data[fsidSecretName]),
		MonitorSecret: string(secrets.Data[monSecretName]),
		AdminSecret:   string(secrets.Data[adminSecretName]),
		Monitors:      map[string]*mon.CephMonitorConfig{},
	}
	logger.Infof("found existing monitor secrets for cluster %s with fsid %s", info.Name, info.FSID)
	return info, nil
}

func (c *Cluster) createMonSecretsAndSave(clientset kubernetes.Interface) (*mon.ClusterInfo, error) {
	logger.Infof("creating mon secrets for a new cluster")
	info, err := mon.CreateClusterInfo(c.factory, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create mon secrets. %+v", err)
	}

	// store the secrets for internal usage of the rook pods
	secrets := map[string]string{
		clusterSecretName: info.Name,
		fsidSecretName:    info.FSID,
		monSecretName:     info.MonitorSecret,
		adminSecretName:   info.AdminSecret,
	}
	secret := &v1.Secret{
		ObjectMeta: v1.ObjectMeta{Name: appName, Namespace: c.Namespace},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	_, err = clientset.CoreV1().Secrets(c.Namespace).Create(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to save mon secrets. %+v", err)
	}

	// store the secret for usage by the storage class
	storageClassSecret := map[string]string{
		"key": info.AdminSecret,
	}
	secret = &v1.Secret{
		ObjectMeta: v1.ObjectMeta{Name: "rook-admin", Namespace: c.Namespace},
		StringData: storageClassSecret,
		Type:       k8sutil.RbdType,
	}
	_, err = clientset.CoreV1().Secrets(c.Namespace).Create(secret)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to save rook-admin secret. %+v", err)
		}
		logger.Infof("rook-admin secret already exists")
	} else {
		logger.Infof("saved rook-admin secret")
	}

	return info, nil
}

func (c *Cluster) startPods(clientset kubernetes.Interface, clusterInfo *mon.ClusterInfo, mons []*MonConfig) error {
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

	clusterInfo.Monitors = map[string]*mon.CephMonitorConfig{}
	for _, m := range running {
		clusterInfo.Monitors[m.Name] = mon.ToCephMon(m.Name, m.Status.PodIP)
	}

	if len(running) == c.Size {
		logger.Infof("pods are already running")
		return nil
	}

	started := 0
	alreadyRunning := 0
	for _, m := range mons {
		monPod := c.makeMonPod(m, clusterInfo, antiAffinity)
		name := monPod.Name
		logger.Debugf("Starting pod: %+v", monPod)
		monPod, err = clientset.CoreV1().Pods(c.Namespace).Create(monPod)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create mon pod %s. %+v", name, err)
			}
			alreadyRunning++
		} else {
			started++
		}

		podIP, err := c.waitForPodToStart(clientset, name)
		if err != nil {
			return fmt.Errorf("failed to start pod %s. %+v", name, err)
		}
		clusterInfo.Monitors[m.Name] = mon.ToCephMon(m.Name, podIP)
	}

	logger.Infof("started %d/%d mons (%d already running)", (started + alreadyRunning), c.Size, alreadyRunning)
	return nil
}

func (c *Cluster) waitForPodToStart(clientset kubernetes.Interface, name string) (string, error) {

	// Poll the status of the pods to see if they are ready
	status := ""
	for i := 0; i < 15; i++ {
		// wait and try again
		logger.Infof("waiting %ds for pod %s to start. status=%s", c.retryDelay, name, status)
		<-time.After(time.Duration(c.retryDelay) * time.Second)

		pod, err := clientset.CoreV1().Pods(c.Namespace).Get(name)
		if err != nil {
			return "", fmt.Errorf("failed to get mon pod %s. %+v", name, err)
		}

		if pod.Status.Phase == v1.PodRunning {
			logger.Infof("pod %s started", name)
			return pod.Status.PodIP, nil
		}
		status = string(pod.Status.Phase)
	}

	return "", fmt.Errorf("timed out waiting for pod %s to start", name)
}

// detect whether we have a big enough cluster to run services on different nodes.
// the anti-affinity will prevent pods of the same type of running on the same node.
func (c *Cluster) getAntiAffinity(clientset kubernetes.Interface) (bool, error) {
	nodeOptions := v1.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	nodes, err := clientset.CoreV1().Nodes().List(nodeOptions)
	if err != nil {
		return false, fmt.Errorf("failed to get nodes in cluster. %+v", err)
	}

	logger.Infof("there are %d nodes available for %d monitors", len(nodes.Items), c.Size)
	return len(nodes.Items) >= c.Size, nil
}
