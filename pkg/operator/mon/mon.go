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

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/pkg/api/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mon")

const (
	appName           = "rook-ceph-mon"
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
	context         *clusterd.Context
	Namespace       string
	Keyring         string
	Version         string
	MasterHost      string
	Size            int
	Paused          bool
	Port            int32
	clusterInfo     *mon.ClusterInfo
	placement       k8sutil.Placement
	maxMonID        int
	waitForStart    bool
	dataDirHostPath string
}

type MonConfig struct {
	Name string
	Port int32
}

func New(context *clusterd.Context, namespace, dataDirHostPath, version string, placement k8sutil.Placement) *Cluster {
	return &Cluster{
		context:         context,
		placement:       placement,
		dataDirHostPath: dataDirHostPath,
		Namespace:       namespace,
		Version:         version,
		Size:            3,
		maxMonID:        -1,
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
		err = c.startPods(mons)
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

func monInQuorum(monitor client.MonMapEntry, quorum []int) bool {
	for _, rank := range quorum {
		if rank == monitor.Rank {
			return true
		}
	}
	return false
}

// Retrieve the ceph cluster info if it already exists.
// If a new cluster create new keys.
func (c *Cluster) initClusterInfo() error {
	// get the cluster secrets
	secrets, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(appName, metav1.GetOptions{})
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

	// make sure we have the connection info generated so connections can happen
	err = c.writeConnectionConfig()
	if err != nil {
		return err
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
		mons = append(mons, &MonConfig{Name: fmt.Sprintf("%s%d", appName, c.maxMonID), Port: int32(mon.Port)})
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
	c.clusterInfo, err = mon.CreateNamedClusterInfo(c.context, "", c.Namespace)
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
		ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: c.Namespace},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	_, err = c.context.Clientset.CoreV1().Secrets(c.Namespace).Create(secret)
	if err != nil {
		return fmt.Errorf("failed to save mon secrets. %+v", err)
	}

	// store the secret for usage by the storage class
	storageClassSecret := map[string]string{
		"key": c.clusterInfo.AdminSecret,
	}
	secret = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "rook-admin", Namespace: c.Namespace},
		StringData: storageClassSecret,
		Type:       k8sutil.RbdType,
	}
	_, err = c.context.Clientset.CoreV1().Secrets(c.Namespace).Create(secret)
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

func (c *Cluster) startPods(mons []*MonConfig) error {
	// schedule the mons on different nodes if we have enough nodes to be unique
	availableNodes, err := c.getAvailableMonNodes()
	if err != nil {
		return fmt.Errorf("failed to get available nodes for mons. %+v", err)
	}

	preexisted := len(c.clusterInfo.Monitors)
	nodeIndex := 0
	for _, m := range mons {
		// pick one of the available nodes where the mon will be assigned
		node := availableNodes[nodeIndex%len(availableNodes)]
		nodeIndex++

		// start the mon
		err := c.startMon(m, node.Name)
		if err != nil {
			return fmt.Errorf("failed to create pod %s. %+v", m.Name, err)
		}

		// wait for the mon to start
		podIP, err := c.waitForPodToStart(m.Name)
		if err != nil {
			return fmt.Errorf("failed to start pod %s. %+v", m.Name, err)
		}
		c.clusterInfo.Monitors[m.Name] = mon.ToCephMon(m.Name, podIP)

		// save the mon config
		err = c.saveMonConfig()
		if err != nil {
			return fmt.Errorf("failed to save endpoints after starting mon %s. %+v", m.Name, err)
		}
	}

	logger.Infof("mons created: %d, preexisted: %d", len(mons), preexisted)

	return c.waitForMonsToJoin(mons)
}

func (c *Cluster) waitForMonsToJoin(mons []*MonConfig) error {
	if !c.waitForStart {
		return nil
	}

	starting := []string{}
	for _, m := range mons {
		starting = append(starting, m.Name)
	}

	// wait for the monitors to join quorum
	err := mon.WaitForQuorumWithMons(c.context, c.clusterInfo.Name, starting)
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
	retryFactor := 3
	for i := 0; i < c.context.MaxRetries*retryFactor; i++ {
		// wait and try again
		if i%retryFactor == 0 {
			logger.Infof("waiting for mon %s to start. status=%s", name, status)
		}
		<-time.After(time.Duration(c.context.RetryDelay/retryFactor) * time.Second)

		options := metav1.ListOptions{LabelSelector: fmt.Sprintf("mon=%s", name)}
		pods, err := c.context.Clientset.CoreV1().Pods(c.Namespace).List(options)
		if err != nil {
			return "", fmt.Errorf("failed to get mon %s pod. %+v", name, err)
		}
		if len(pods.Items) == 0 {
			logger.Infof("%s pod not yet created", name)
			continue
		}
		if len(pods.Items) > 1 {
			logger.Warningf("more than one mon pod found for %s", name)
		}

		pod := pods.Items[0]
		if pod.Status.Phase == v1.PodRunning {
			logger.Infof("pod %s started", pod.Name)
			return pod.Status.PodIP, nil
		}
		status = string(pod.Status.Phase)
	}

	return "", fmt.Errorf("timed out waiting for pod %s to start", name)
}

func (c *Cluster) saveMonConfig() error {
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        monConfigMapName,
			Namespace:   c.Namespace,
			Annotations: map[string]string{},
		},
	}
	configMap.Data = map[string]string{
		monEndpointKey: mon.FlattenMonEndpoints(c.clusterInfo.Monitors),
		maxMonIDKey:    strconv.Itoa(c.maxMonID),
	}

	_, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(configMap)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create mon endpoint config map. %+v", err)
		}

		logger.Debugf("updating config map %s that already exists", configMap.Name)
		if _, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Update(configMap); err != nil {
			return fmt.Errorf("failed to update mon endpoint config map. %+v", err)
		}
	}

	logger.Infof("saved mon endpoints to config map %+v", configMap.Data)

	// write the latest config to the config dir
	if err := c.writeConnectionConfig(); err != nil {
		return fmt.Errorf("failed to write connection config for new mons. %+v", err)
	}

	return nil
}

func (c *Cluster) writeConnectionConfig() error {
	// write the latest config to the config dir
	if err := mon.GenerateAdminConnectionConfig(c.context, c.clusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config. %+v", err)
	}

	return nil
}

func (c *Cluster) loadMonConfig() error {
	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(monConfigMapName, metav1.GetOptions{})
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

// detect the nodes that are available for new mons to start.
func (c *Cluster) getAvailableMonNodes() ([]v1.Node, error) {
	nodeOptions := metav1.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	nodes, err := c.context.Clientset.CoreV1().Nodes().List(nodeOptions)
	if err != nil {
		return nil, err
	}
	logger.Infof("there are %d nodes available for mons (existing mons=%d)", len(nodes.Items), len(c.clusterInfo.Monitors))

	// get the nodes that have mons assigned
	nodesInUse, err := c.getNodesWithMons()
	if err != nil {
		logger.Warningf("could not get nodes with mons. %+v", err)
		nodesInUse = util.NewSet()
	}

	// choose nodes for the new mons that don't have mons currently
	availableNodes := []v1.Node{}
	for _, node := range nodes.Items {
		if !nodesInUse.Contains(node.Name) && validNode(node, c.placement) {
			availableNodes = append(availableNodes, node)
		}
	}
	logger.Infof("Found %d running nodes without mons", len(availableNodes))

	// if all nodes already have mons, just add all nodes to be available
	if len(availableNodes) == 0 {
		logger.Infof("All nodes are running mons. Adding all %d nodes to the availability.", len(nodes.Items))
		for _, node := range nodes.Items {
			if validNode(node, c.placement) {
				availableNodes = append(availableNodes, node)
			}
		}
	}
	if len(availableNodes) == 0 {
		return nil, fmt.Errorf("no nodes are available for mons")
	}

	return availableNodes, nil
}

func validNode(node v1.Node, placement k8sutil.Placement) bool {
	// a node cannot be disabled
	if node.Spec.Unschedulable {
		return false
	}

	// a node matches the NodeAffinity configuration
	// ignoring `PreferredDuringSchedulingIgnoredDuringExecution` terms: they
	// should not be used to judge a node unusable
	if placement.NodeAffinity != nil && placement.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		nodeMatches := false
		for _, req := range placement.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			nodeSelector, err := v1.NodeSelectorRequirementsAsSelector(req.MatchExpressions)
			if err != nil {
				logger.Infof("failed to parse MatchExpressions: %+v, regarding as not match.", req.MatchExpressions)
				return false
			}
			if nodeSelector.Matches(labels.Set(node.Labels)) {
				nodeMatches = true
				break
			}
		}
		if !nodeMatches {
			return false
		}
	}

	// a node is tainted and cannot be tolerated
	for _, taint := range node.Spec.Taints {
		isTolerated := false
		for _, toleration := range placement.Tolerations {
			if toleration.ToleratesTaint(&taint) {
				isTolerated = true
				break
			}
		}
		if !isTolerated {
			return false
		}
	}

	// a node must be Ready
	for _, c := range node.Status.Conditions {
		if c.Type == v1.NodeReady {
			return true
		}
	}
	logger.Infof("node %s is not ready. %+v", node.Name, node.Status.Conditions)
	return false
}

func (c *Cluster) getNodesWithMons() (*util.Set, error) {
	options := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appName)}
	pods, err := c.context.Clientset.CoreV1().Pods(c.Namespace).List(options)
	if err != nil {
		return nil, err
	}
	nodes := util.NewSet()
	for _, pod := range pods.Items {
		hostname := pod.Spec.NodeSelector[metav1.LabelHostname]
		logger.Debugf("mon pod on node %s", hostname)
		nodes.Add(hostname)
	}
	return nodes, nil
}

func (c *Cluster) startMon(m *MonConfig, nodeName string) error {
	rs := c.makeReplicaSet(m, nodeName)
	logger.Debugf("Starting mon: %+v", rs.Name)
	_, err := c.context.Clientset.Extensions().ReplicaSets(c.Namespace).Create(rs)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create mon %s. %+v", m.Name, err)
		}
		logger.Infof("replicaset %s already exists", m.Name)
	}
	return nil
}
