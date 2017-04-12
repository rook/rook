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
	"strconv"
	"strings"
	"time"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
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
	monConfigMapName  = "mon-config"
	monEndpointKey    = "endpoints"
	maxMonIDKey       = "maxMonId"
)

type Cluster struct {
	Name            string
	Namespace       string
	Keyring         string
	Version         string
	MasterHost      string
	Size            int
	Paused          bool
	AntiAffinity    bool
	Port            int32
	clientset       kubernetes.Interface
	factory         client.ConnectionFactory
	retryDelay      int
	maxRetries      int
	clusterInfo     *mon.ClusterInfo
	maxMonID        int
	configDir       string
	waitForStart    bool
	dataDirHostPath string
}

type MonConfig struct {
	Name string
	Port int32
}

func New(clientset kubernetes.Interface, factory client.ConnectionFactory, name, namespace, dataDirHostPath, version string) *Cluster {
	return &Cluster{
		clientset:       clientset,
		factory:         factory,
		dataDirHostPath: dataDirHostPath,
		Name:            name,
		Namespace:       namespace,
		Version:         version,
		Size:            3,
		AntiAffinity:    true,
		retryDelay:      6,
		maxRetries:      15,
		maxMonID:        -1,
		configDir:       k8sutil.DataDir,
		waitForStart:    true,
	}
}

func (c *Cluster) Start() (*mon.ClusterInfo, error) {
	logger.Infof("start running mons")

	err := c.initClusterInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ceph cluster info. %+v", err)
	}

	if len(c.clusterInfo.Monitors) == 0 {
		// Start the initial monitors at startup
		mons := c.getExpectedMonConfig()
		err = c.startPods(nil, mons)
		if err != nil {
			return nil, fmt.Errorf("failed to start mon pods. %+v", err)
		}
	} else {
		// Check the health of a previously started cluster
		err = c.CheckHealth()
		if err != nil {
			logger.Warningf("failed to check mon health %+v. %+v", c.clusterInfo.Monitors, err)
		}
	}

	return c.clusterInfo, nil
}

func (c *Cluster) CheckHealth() error {
	// update the config map if the pod ips changed
	// must retry since during startup of pods they might take some time to initialize
	k8sutil.Retry(time.Duration(c.retryDelay)*time.Second, c.maxRetries, func() (bool, error) {
		// TODO: There is more work to get the reboot functional. The mons are not
		// happy if their ip address changes. They expect a constant id.
		err := c.updateConfigMapIfPodIPsChanged()
		if err != nil {
			logger.Infof("unable to check on mon pods. %+v", err)
			return false, nil
		}
		return true, nil
	})

	// connect to the mons
	context := &clusterd.Context{ConfigDir: c.configDir}
	conn, err := mon.ConnectToClusterAsAdmin(context, c.factory, c.clusterInfo)
	if err != nil {
		return fmt.Errorf("cannot connect to cluster. %+v", err)
	}
	defer conn.Shutdown()

	// get the status and check for quorum
	status, err := client.GetMonStatus(conn)
	if err != nil {
		return fmt.Errorf("failed to get mon status. %+v", err)
	}

	for _, monitor := range status.MonMap.Mons {
		inQuorum := monInQuorum(monitor, status.Quorum)
		if inQuorum {
			logger.Debugf("mon %s found in quorum", monitor.Name)
		} else {
			logger.Warningf("mon %s NOT found in quroum. %+v", monitor.Name, status.Quorum)

			err = c.failoverMon(conn, monitor.Name)
			if err != nil {
				logger.Errorf("failed to failover mon %s. %+v", monitor.Name, err)
			}
		}
	}

	return nil
}

func (c *Cluster) updateConfigMapIfPodIPsChanged() error {
	pods, err := c.getPods()
	if err != nil {
		return fmt.Errorf("failed to check if pod ips changed. %+v", err)
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("where are the mon pods?")
	}

	logger.Debugf("there are %d mon pods. checking the podIPs.", len(pods.Items))
	updated := false
	for _, pod := range pods.Items {
		if pod.Status.PodIP == "" {
			return fmt.Errorf("no podIP for mon %s", pod.Name)
		}
		if pod.Status.Phase != v1.PodRunning {
			return fmt.Errorf("pod %s is not running. phase=%v", pod.Name, pod.Status.Phase)
		}
		m := mon.ToCephMon(pod.Name, pod.Status.PodIP)
		existing, ok := c.clusterInfo.Monitors[pod.Name]
		if !ok || existing.Endpoint == m.Endpoint {
			// the endpoint does not need to be updated
			logger.Debugf("Did not need to update pod %s with endpoint %+v. ok=%t, m=%+v", pod.Name, pod.Status.PodIP, ok, m)
			continue
		}

		logger.Infof("updating mon %s endpoint from %s to %s", pod.Name, existing.Endpoint, m.Endpoint)
		c.clusterInfo.Monitors[pod.Name] = m
		updated = true
	}

	if updated {
		return c.saveMonConfig()
	}

	logger.Debugf("no update to mon pod ips")
	return nil
}

func monInQuorum(monitor client.MonMapEntry, quorum []int) bool {
	for _, rank := range quorum {
		if rank == monitor.Rank {
			return true
		}
	}
	return false
}

func (c *Cluster) failoverMon(conn client.Connection, name string) error {
	logger.Infof("Failing over monitor %s", name)

	// Start a new monitor
	c.maxMonID++
	mons := []*MonConfig{&MonConfig{Name: fmt.Sprintf("mon%d", c.maxMonID), Port: int32(mon.Port)}}
	logger.Infof("starting new mon %s", mons[0].Name)
	err := c.startPods(conn, mons)
	if err != nil {
		return fmt.Errorf("failed to start new mon %s. %+v", mons[0].Name, err)
	}

	// Remove the mon pod if it is still there
	var gracePeriod int64
	options := &v1.DeleteOptions{GracePeriodSeconds: &gracePeriod}
	err = c.clientset.CoreV1().Pods(c.Namespace).Delete(name, options)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Infof("dead mon pod %s was already gone", name)
		} else {
			return fmt.Errorf("failed to remove dead mon pod %s. %+v", name, err)
		}
	}

	// Remove the bad monitor from quorum
	err = mon.RemoveMonitorFromQuorum(conn, name)
	if err != nil {
		return fmt.Errorf("failed to remove mon %s from quorum. %+v", name, err)
	}
	delete(c.clusterInfo.Monitors, name)
	err = c.saveMonConfig()
	if err != nil {
		return fmt.Errorf("failed to save mon config after failing mon %s. %+v", name, err)
	}

	return nil
}

// Retrieve the ceph cluster info if it already exists.
// If a new cluster create new keys.
func (c *Cluster) initClusterInfo() error {
	// get the cluster secrets
	secrets, err := c.clientset.CoreV1().Secrets(c.Namespace).Get(appName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get mon secrets. %+v", err)
		}

		err = c.createMonSecretsAndSave()
		if err != nil {
			return err
		}
	} else {
		c.clusterInfo = &mon.ClusterInfo{
			Name:          string(secrets.Data[clusterSecretName]),
			FSID:          string(secrets.Data[fsidSecretName]),
			MonitorSecret: string(secrets.Data[monSecretName]),
			AdminSecret:   string(secrets.Data[adminSecretName]),
		}
		logger.Debugf("found existing monitor secrets for cluster %s", c.clusterInfo.Name)
	}

	// get the existing monitor config
	err = c.loadMonConfig()
	if err != nil {
		return fmt.Errorf("failed to get mon config. %+v", err)
	}
	return nil
}

func (c *Cluster) getExpectedMonConfig() []*MonConfig {
	mons := []*MonConfig{}

	// initialize the mon pod info for mons that have been previously created
	for _, monitor := range c.clusterInfo.Monitors {
		mons = append(mons, &MonConfig{Name: monitor.Name, Port: int32(mon.Port)})
	}

	// initialize mon info if we don't have enough mons (at first startup)
	for i := len(c.clusterInfo.Monitors); i < c.Size; i++ {
		c.maxMonID++
		mons = append(mons, &MonConfig{Name: fmt.Sprintf("mon%d", c.maxMonID), Port: int32(mon.Port)})
	}

	return mons
}

// get the ID of a monitor from its name
func getMonID(name string) (int, error) {
	if strings.Index(name, "mon") != 0 || len(name) < 4 {
		return -1, fmt.Errorf("unexpected mon name")
	}
	id, err := strconv.Atoi(name[3:])
	if err != nil {
		return -1, err
	}
	return id, nil
}

func (c *Cluster) createMonSecretsAndSave() error {
	logger.Infof("creating mon secrets for a new cluster")
	var err error
	c.clusterInfo, err = mon.CreateNamedClusterInfo(c.factory, "", c.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create mon secrets. %+v", err)
	}

	// store the secrets for internal usage of the rook pods
	secrets := map[string]string{
		clusterSecretName: c.clusterInfo.Name,
		fsidSecretName:    c.clusterInfo.FSID,
		monSecretName:     c.clusterInfo.MonitorSecret,
		adminSecretName:   c.clusterInfo.AdminSecret,
	}
	secret := &v1.Secret{
		ObjectMeta: v1.ObjectMeta{Name: appName, Namespace: c.Namespace},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	_, err = c.clientset.CoreV1().Secrets(c.Namespace).Create(secret)
	if err != nil {
		return fmt.Errorf("failed to save mon secrets. %+v", err)
	}

	// store the secret for usage by the storage class
	storageClassSecret := map[string]string{
		"key": c.clusterInfo.AdminSecret,
	}
	secret = &v1.Secret{
		ObjectMeta: v1.ObjectMeta{Name: "rook-admin", Namespace: c.Namespace},
		StringData: storageClassSecret,
		Type:       k8sutil.RbdType,
	}
	_, err = c.clientset.CoreV1().Secrets(c.Namespace).Create(secret)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to save rook-admin secret. %+v", err)
		}
		logger.Infof("rook-admin secret already exists")
	} else {
		logger.Infof("saved rook-admin secret")
	}

	return nil
}

func (c *Cluster) startPods(conn client.Connection, mons []*MonConfig) error {
	// schedule the mons on different nodes if we have enough nodes to be unique
	antiAffinity, err := c.getAntiAffinity()
	if err != nil {
		return fmt.Errorf("failed to get antiaffinity. %+v", err)
	}

	preexisted := len(c.clusterInfo.Monitors)
	created := 0
	alreadyRunning := 0
	for _, m := range mons {
		monPod := c.makeMonPod(m, antiAffinity)
		logger.Debugf("Starting pod: %+v", monPod)
		name := monPod.Name
		_, err = c.clientset.CoreV1().Pods(c.Namespace).Create(monPod)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create mon pod %s. %+v", name, err)
			}
			logger.Infof("pod %s already exists", name)
			alreadyRunning++
		} else {
			created++
		}

		podIP, err := c.waitForPodToStart(name)
		if err != nil {
			return fmt.Errorf("failed to start pod %s. %+v", name, err)
		}
		c.clusterInfo.Monitors[m.Name] = mon.ToCephMon(m.Name, podIP)

		err = c.saveMonConfig()
		if err != nil {
			return fmt.Errorf("failed to save endpoints after starting mon %s. %+v", m.Name, err)
		}
	}

	logger.Infof("mons created: %d, alreadyRunning: %d, preexisted: %d", created, alreadyRunning, preexisted)

	return c.waitForMonsToJoin(conn, mons)
}

func (c *Cluster) waitForMonsToJoin(conn client.Connection, mons []*MonConfig) error {
	if !c.waitForStart {
		return nil
	}

	// initialize a connection if it is not already connected
	if conn == nil {
		context := &clusterd.Context{ConfigDir: k8sutil.DataDir}
		var err error
		conn, err = mon.ConnectToClusterAsAdmin(context, c.factory, c.clusterInfo)
		if err != nil {
			return fmt.Errorf("cannot connect to cluster. %+v", err)
		}
		defer conn.Shutdown()
	}

	starting := []string{}
	for _, m := range mons {
		starting = append(starting, m.Name)
	}

	// wait for the monitors to join quorum
	err := mon.WaitForQuorumWithConnection(conn, starting)
	if err != nil {
		return fmt.Errorf("failed to wait for mon quorum. %+v", err)
	}

	return nil
}

func (c *Cluster) waitForPodToStart(name string) (string, error) {
	if !c.waitForStart {
		return "", nil
	}

	// Poll the status of the pods to see if they are ready
	status := ""
	for i := 0; i < c.maxRetries; i++ {
		// wait and try again
		logger.Infof("waiting %ds for pod %s to start. status=%s", c.retryDelay, name, status)
		<-time.After(time.Duration(c.retryDelay) * time.Second)

		pod, err := c.clientset.CoreV1().Pods(c.Namespace).Get(name)
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

func (c *Cluster) saveMonConfig() error {
	configMap := &v1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:        monConfigMapName,
			Namespace:   c.Namespace,
			Annotations: map[string]string{},
		},
	}
	configMap.Data = map[string]string{
		monEndpointKey: mon.FlattenMonEndpoints(c.clusterInfo.Monitors),
		maxMonIDKey:    strconv.Itoa(c.maxMonID),
	}

	_, err := c.clientset.CoreV1().ConfigMaps(c.Namespace).Create(configMap)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create mon endpoint config map. %+v", err)
		}

		logger.Debugf("updating config map %s that already exists", configMap.Name)
		if _, err = c.clientset.CoreV1().ConfigMaps(c.Namespace).Update(configMap); err != nil {
			return fmt.Errorf("failed to update mon endpoint config map. %+v", err)
		}
	}

	logger.Infof("saved mon endpoints to config map %s", configMap.Name)
	return nil
}

func (c *Cluster) loadMonConfig() error {
	cm, err := c.clientset.CoreV1().ConfigMaps(c.Namespace).Get(monConfigMapName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		// If the config map was not found, initialize the empty set of monitors
		c.maxMonID = -1
		c.clusterInfo.Monitors = map[string]*mon.CephMonitorConfig{}
		return c.saveMonConfig()
	}

	// Parse the monitor List
	if info, ok := cm.Data[monEndpointKey]; ok {
		c.clusterInfo.Monitors = mon.ParseMonEndpoints(info)
	} else {
		c.clusterInfo.Monitors = map[string]*mon.CephMonitorConfig{}
	}

	// Parse the max monitor id
	if id, ok := cm.Data[maxMonIDKey]; ok {
		c.maxMonID, err = strconv.Atoi(id)
		if err != nil {
			logger.Errorf("invalid max mon id %s. %+v", id, err)
		}
	}

	// Make sure the max id is consistent with the current monitors
	for _, m := range c.clusterInfo.Monitors {
		id, _ := getMonID(m.Name)
		if c.maxMonID < id {
			c.maxMonID = id
		}
	}

	logger.Infof("loaded: maxMonID=%d, mons=%+v", c.maxMonID, c.clusterInfo.Monitors)
	return nil
}

// detect whether we have a big enough cluster to run services on different nodes.
// the anti-affinity will prevent pods of the same type of running on the same node.
func (c *Cluster) getAntiAffinity() (bool, error) {
	nodeOptions := v1.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	nodes, err := c.clientset.CoreV1().Nodes().List(nodeOptions)
	if err != nil {
		return false, fmt.Errorf("failed to get nodes in cluster. %+v", err)
	}

	logger.Infof("there are %d nodes available for %d monitors", len(nodes.Items), c.Size)
	return len(nodes.Items) >= c.Size, nil
}
