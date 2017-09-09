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

// Package mon for the Ceph monitors.
package mon

import (
	"encoding/json"
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
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	helper "k8s.io/kubernetes/pkg/api/v1/helper"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mon")

const (
	// EndpointConfigMapName is the name of the configmap with mon endpoints
	EndpointConfigMapName = "rook-ceph-mon-endpoints"
	// EndpointDataKey is the name of the key inside the mon configmap to get the endpoints
	EndpointDataKey = "data"
	// MaxMonIDKey is the name of the max mon id used
	MaxMonIDKey = "maxMonId"
	// MappingKey is the name of the mapping for the mon->node and node->port
	MappingKey = "mapping"

	appName           = "rook-ceph-mon"
	monNodeAttr       = "mon_node"
	monClusterAttr    = "mon_cluster"
	tprName           = "mon.rook.io"
	fsidSecretName    = "fsid"
	monSecretName     = "mon-secret"
	adminSecretName   = "admin-secret"
	clusterSecretName = "cluster-name"
)

// Cluster is for the cluster of monitors
type Cluster struct {
	context             *clusterd.Context
	Namespace           string
	Keyring             string
	Version             string
	MasterHost          string
	Size                int
	Paused              bool
	Port                int32
	clusterInfo         *mon.ClusterInfo
	placement           k8sutil.Placement
	maxMonID            int
	waitForStart        bool
	dataDirHostPath     string
	monPodRetryInterval time.Duration
	monPodTimeout       time.Duration
	monTimeoutList      map[string]time.Time
	HostNetwork         bool
	mapping             *mapping
}

// monConfig for a single monitor
type monConfig struct {
	Name     string
	PublicIP string
	Port     int32
}

// mapping mon node and port mapping
type mapping struct {
	Node map[string]*nodeInfo `json:"node"`
	Port map[string]int32     `json:"port"`
}

type nodeInfo struct {
	Name    string
	Address string
}

// New creates an instance of a mon cluster
func New(context *clusterd.Context, namespace, dataDirHostPath, version string, placement k8sutil.Placement, hostNetwork bool) *Cluster {
	return &Cluster{
		context:             context,
		placement:           placement,
		dataDirHostPath:     dataDirHostPath,
		Namespace:           namespace,
		Version:             version,
		Size:                3,
		maxMonID:            -1,
		waitForStart:        true,
		monPodRetryInterval: 6 * time.Second,
		monPodTimeout:       5 * time.Minute,
		monTimeoutList:      map[string]time.Time{},
		HostNetwork:         hostNetwork,
		mapping: &mapping{
			Node: map[string]*nodeInfo{},
			Port: map[string]int32{},
		},
	}
}

// Start the mon cluster
func (c *Cluster) Start() (*mon.ClusterInfo, error) {
	logger.Infof("start running mons")

	if err := c.initClusterInfo(); err != nil {
		return nil, fmt.Errorf("failed to initialize ceph cluster info. %+v", err)
	}

	if len(c.clusterInfo.Monitors) < c.Size {
		// init the mons config
		mons := c.initMonConfig(c.Size)

		// Assign the pods to nodes
		if err := c.assignMons(mons); err != nil {
			return nil, fmt.Errorf("failed to assign pods to mons. %+v", err)
		}

		// Start one monitor at a time
		for i := len(c.clusterInfo.Monitors); i < c.Size; i++ {
			// Init the mon IPs
			if err := c.initMonIPs(mons[0 : i+1]); err != nil {
				return nil, fmt.Errorf("failed to init mon services. %+v", err)
			}

			// save the mon config after we have "initiated the IPs"
			if err := c.saveMonConfig(); err != nil {
				return nil, fmt.Errorf("failed to save mons. %+v", err)
			}

			// make sure we have the connection info generated so connections can happen
			if err := c.writeConnectionConfig(); err != nil {
				return nil, err
			}

			// Start pods
			if err := c.startPods(mons[0 : i+1]); err != nil {
				return nil, fmt.Errorf("failed to start mon pods. %+v", err)
			}
		}
		logger.Debugf("mon endpoints used are: %s", c.clusterInfo.MonEndpoints())
	} else {
		// Check the health of a previously started cluster
		if err := c.checkHealth(); err != nil {
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

		if err = c.createMonSecretsAndSave(); err != nil {
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
	if err = c.loadMonConfig(); err != nil {
		return fmt.Errorf("failed to get mon config. %+v", err)
	}

	// make sure we have the connection info generated after loading it in the first mon init step
	if err = c.writeConnectionConfig(); err != nil {
		return err
	}

	return nil
}

func (c *Cluster) initMonConfig(size int) []*monConfig {
	mons := []*monConfig{}

	// initialize the mon pod info for mons that have been previously created
	for _, monitor := range c.clusterInfo.Monitors {
		mons = append(mons, &monConfig{Name: monitor.Name, Port: int32(mon.DefaultPort)})
	}

	// initialize mon info if we don't have enough mons (at first startup)
	for i := len(c.clusterInfo.Monitors); i < size; i++ {
		c.maxMonID++
		mons = append(mons, &monConfig{Name: fmt.Sprintf("%s%d", appName, c.maxMonID), Port: int32(mon.DefaultPort)})
	}

	return mons
}

func (c *Cluster) initMonIPs(mons []*monConfig) error {
	for _, m := range mons {
		if c.HostNetwork {
			logger.Infof("setting mon endpoints for hostnetwork mode")
			node, ok := c.mapping.Node[m.Name]
			if !ok {
				return fmt.Errorf("mon doesn't exit in assignment map")
			}
			m.PublicIP = node.Address
		} else {
			serviceIP, err := c.createService(m)
			if err != nil {
				return fmt.Errorf("failed to create mon service. %+v", err)
			}
			m.PublicIP = serviceIP
		}
		c.clusterInfo.Monitors[m.Name] = mon.ToCephMon(m.Name, m.PublicIP, m.Port)
	}

	return nil
}

// get the ID of a monitor from its name
func getMonID(name string) (int, error) {
	if strings.Index(name, appName) != 0 || len(name) < len(appName) {
		return -1, fmt.Errorf("unexpected mon name")
	}
	id, err := strconv.Atoi(name[len(appName):])
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

func (c *Cluster) createService(mon *monConfig) (string, error) {
	labels := c.getLabels(mon.Name)
	s := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   mon.Name,
			Labels: labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       mon.Name,
					Port:       mon.Port,
					TargetPort: intstr.FromInt(int(mon.Port)),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
	if c.HostNetwork {
		s.Spec.ClusterIP = v1.ClusterIPNone
	}

	s, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(s)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return "", fmt.Errorf("failed to create mon service. %+v", err)
		}
		s, err = c.context.Clientset.CoreV1().Services(c.Namespace).Get(mon.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to get mon %s service ip. %+v", mon.Name, err)
		}
	}

	if s == nil {
		logger.Warningf("service ip not found for mon %s. this better be a test", mon.Name)
		return "", nil
	}

	logger.Infof("mon %s running at %s:%d", mon.Name, s.Spec.ClusterIP, mon.Port)
	return s.Spec.ClusterIP, nil
}

func (c *Cluster) assignMons(mons []*monConfig) error {
	// schedule the mons on different nodes if we have enough nodes to be unique
	availableNodes, err := c.getAvailableMonNodes()
	if err != nil {
		return fmt.Errorf("failed to get available nodes for mons. %+v", err)
	}

	nodeIndex := 0
	for _, m := range mons {
		if _, ok := c.mapping.Node[m.Name]; ok {
			logger.Debugf("mon %s already assigned to a node, no need to assign", m.Name)
			continue
		}

		// when the available nodes are >= than the mon size we just begin taking
		// "node0" through nodeX (X = c.Size-1)
		var node v1.Node
		// pick one of the available nodes where the mon will be assigned
		node = availableNodes[nodeIndex%len(availableNodes)]
		logger.Debugf("mon %s assigned to node %s", m.Name, node.Name)
		nodeInfo, err := getNodeInfoFromNode(node)
		if err != nil {
			return fmt.Errorf("couldn't get node info from node %s. %+v", node.Name, err)
		}
		// when hostNetwork is used check if we need to increase the port of the node
		if c.HostNetwork {
			if _, ok := c.mapping.Port[node.Name]; ok {
				// when the node was already chosen increase port by 1 and set
				// assignment and that the node was chosen
				m.Port = c.mapping.Port[node.Name] + int32(1)
			}
			c.mapping.Port[node.Name] = m.Port
		}
		c.mapping.Node[m.Name] = nodeInfo
		nodeIndex++
	}

	logger.Debug("assigned mons to nodes")
	return nil
}

func getNodeInfoFromNode(n v1.Node) (*nodeInfo, error) {
	nr := &nodeInfo{}
	nr.Name = n.Name
	for _, ip := range n.Status.Addresses {
		if ip.Type == v1.NodeExternalIP || ip.Type == v1.NodeInternalIP {
			logger.Debugf("using IP %s for node %s", ip.Address, n.Name)
			nr.Address = ip.Address
			break
		}
	}
	if nr.Address == "" {
		return nil, fmt.Errorf("no IP given for node %s", nr.Name)
	}
	return nr, nil
}

func (c *Cluster) startPods(mons []*monConfig) error {
	for _, m := range mons {
		node, _ := c.mapping.Node[m.Name]

		// start the mon replicaset/pod
		err := c.startMon(m, node.Name)
		if err != nil {
			return fmt.Errorf("failed to create pod %s. %+v", m.Name, err)
		}
	}

	logger.Infof("mons created: %d", len(mons))
	return c.waitForMonsToJoin(mons)
}

func (c *Cluster) waitForMonsToJoin(mons []*monConfig) error {
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

func (c *Cluster) saveMonConfig() error {
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        EndpointConfigMapName,
			Namespace:   c.Namespace,
			Annotations: map[string]string{},
		},
	}

	monMapping, err := json.Marshal(c.mapping)
	if err != nil {
		return fmt.Errorf("failed to marshal mon mapping. %+v", err)
	}

	configMap.Data = map[string]string{
		EndpointDataKey: mon.FlattenMonEndpoints(c.clusterInfo.Monitors),
		MaxMonIDKey:     strconv.Itoa(c.maxMonID),
		MappingKey:      string(monMapping),
	}

	if _, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(configMap); err != nil {
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
	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		// If the config map was not found, initialize the empty set of monitors
		c.mapping = &mapping{
			Node: map[string]*nodeInfo{},
			Port: map[string]int32{},
		}
		c.clusterInfo.Monitors = map[string]*mon.CephMonitorConfig{}
		c.maxMonID = -1
		return c.saveMonConfig()
	}

	// Parse the monitor List
	if info, ok := cm.Data[EndpointDataKey]; ok {
		c.clusterInfo.Monitors = mon.ParseMonEndpoints(info)
	} else {
		c.clusterInfo.Monitors = map[string]*mon.CephMonitorConfig{}
	}

	// Parse the max monitor id
	if id, ok := cm.Data[MaxMonIDKey]; ok {
		c.maxMonID, err = strconv.Atoi(id)
		if err != nil {
			logger.Errorf("invalid max mon id %s. %+v", id, err)
		}
	}

	// Unmarshal mon mapping (node, port)
	if mappingStr, ok := cm.Data[MappingKey]; ok && mappingStr != "" {
		if err := json.Unmarshal([]byte(mappingStr), &c.mapping); err != nil {
			logger.Errorf("invalid mapping json. json=%s; %+v", mappingStr, err)
			c.mapping = &mapping{
				Node: map[string]*nodeInfo{},
				Port: map[string]int32{},
			}
		}
	} else {
		c.mapping = &mapping{
			Node: map[string]*nodeInfo{},
			Port: map[string]int32{},
		}
	}

	// Make sure the max id is consistent with the current monitors
	for _, m := range c.clusterInfo.Monitors {
		id, _ := getMonID(m.Name)
		if c.maxMonID < id {
			c.maxMonID = id
		}
	}

	logger.Infof("loaded: maxMonID=%d, mons=%+v mapping=%+v", c.maxMonID, c.clusterInfo.Monitors, c.mapping)
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
	logger.Infof("there are %d nodes available for %d mons", len(nodes.Items), len(c.clusterInfo.Monitors))

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
			nodeSelector, err := helper.NodeSelectorRequirementsAsSelector(req.MatchExpressions)
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
		hostname := pod.Spec.NodeSelector[apis.LabelHostname]
		logger.Debugf("mon pod on node %s", hostname)
		nodes.Add(hostname)
	}
	return nodes, nil
}

func (c *Cluster) startMon(m *monConfig, nodeName string) error {
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
