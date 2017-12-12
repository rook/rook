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
	"k8s.io/apimachinery/pkg/util/intstr"
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
	mapping             *Mapping
	resources           v1.ResourceRequirements
}

// monConfig for a single monitor
type monConfig struct {
	Name     string
	PublicIP string
	Port     int32
}

// Mapping mon node and port mapping
type Mapping struct {
	Node map[string]*NodeInfo `json:"node"`
	Port map[string]int32     `json:"port"`
}

// NodeInfo contains name and address of a node
type NodeInfo struct {
	Name     string
	Address  string
	Hostname string
}

// New creates an instance of a mon cluster
func New(context *clusterd.Context, namespace, dataDirHostPath, version string, size int, placement k8sutil.Placement, hostNetwork bool, resources v1.ResourceRequirements) *Cluster {
	return &Cluster{
		context:             context,
		placement:           placement,
		dataDirHostPath:     dataDirHostPath,
		Namespace:           namespace,
		Version:             version,
		Size:                size,
		maxMonID:            -1,
		waitForStart:        true,
		monPodRetryInterval: 6 * time.Second,
		monPodTimeout:       5 * time.Minute,
		monTimeoutList:      map[string]time.Time{},
		HostNetwork:         hostNetwork,
		mapping: &Mapping{
			Node: map[string]*NodeInfo{},
			Port: map[string]int32{},
		},
		resources: resources,
	}
}

// Start the mon cluster
func (c *Cluster) Start() error {
	logger.Infof("start running mons")

	if err := c.initClusterInfo(); err != nil {
		return fmt.Errorf("failed to initialize ceph cluster info. %+v", err)
	}

	if len(c.clusterInfo.Monitors) < c.Size {
		c.startMons()
	} else {
		// Check the health of a previously started cluster
		if err := c.checkHealth(); err != nil {
			logger.Warningf("failed to check mon health %+v. %+v", c.clusterInfo.Monitors, err)
		}
	}

	return nil
}

func (c *Cluster) startMons() error {
	// init the mons config
	mons := c.initMonConfig(c.Size)

	// Assign the pods to nodes
	if err := c.assignMons(mons); err != nil {
		return fmt.Errorf("failed to assign pods to mons. %+v", err)
	}

	// Start one monitor at a time
	for i := len(c.clusterInfo.Monitors); i < c.Size; i++ {
		// Init the mon IPs
		if err := c.initMonIPs(mons[0 : i+1]); err != nil {
			return fmt.Errorf("failed to init mon services. %+v", err)
		}

		// save the mon config after we have "initiated the IPs"
		if err := c.saveMonConfig(); err != nil {
			return fmt.Errorf("failed to save mons. %+v", err)
		}

		// make sure we have the connection info generated so connections can happen
		if err := WriteConnectionConfig(c.context, c.clusterInfo); err != nil {
			return err
		}

		// Start pods
		if err := c.startPods(mons[0 : i+1]); err != nil {
			return fmt.Errorf("failed to start mon pods. %+v", err)
		}
	}

	logger.Debugf("mon endpoints used are: %s", c.clusterInfo.MonEndpoints())
	return nil
}

// Retrieve the ceph cluster info if it already exists.
// If a new cluster create new keys.
func (c *Cluster) initClusterInfo() error {

	var err error
	// get the cluster info from secret
	c.clusterInfo, c.maxMonID, c.mapping, err = LoadClusterInfo(c.context, c.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get cluster info. %+v", err)
	}

	// save cluster monitor config
	if err = c.saveMonConfig(); err != nil {
		return fmt.Errorf("failed to save mons. %+v", err)
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
	availableNodes, err := c.getMonNodes()
	if err != nil {
		return fmt.Errorf("failed to get available nodes for mons. %+v", err)
	}

	nodeIndex := 0
	for _, m := range mons {
		if _, ok := c.mapping.Node[m.Name]; ok {
			logger.Debugf("mon %s already assigned to a node, no need to assign", m.Name)
			continue
		}

		// pick one of the available nodes where the mon will be assigned
		node := availableNodes[nodeIndex%len(availableNodes)]
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

func getNodeInfoFromNode(n v1.Node) (*NodeInfo, error) {
	nr := &NodeInfo{}
	nr.Name = n.Name
	for _, ip := range n.Status.Addresses {
		if ip.Type == v1.NodeExternalIP || ip.Type == v1.NodeInternalIP {
			logger.Debugf("using IP %s for node %s", ip.Address, n.Name)
			nr.Address = ip.Address
			break
		}
	}
	nr.Hostname = n.Labels[apis.LabelHostname]
	if nr.Address == "" {
		return nil, fmt.Errorf("no IP given for node %s", nr.Name)
	}
	return nr, nil
}

func (c *Cluster) startPods(mons []*monConfig) error {
	for _, m := range mons {
		node, _ := c.mapping.Node[m.Name]
		nodeId := getNodeId(node)
		// start the mon replicaset/pod
		err := c.startMon(m, nodeId)
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
	err := waitForQuorumWithMons(c.context, c.clusterInfo.Name, starting)
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
	if err := WriteConnectionConfig(c.context, c.clusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config for new mons. %+v", err)
	}

	return nil
}

// detect the nodes that are available for new mons to start.
func (c *Cluster) getMonNodes() ([]v1.Node, error) {
	availableNodes, nodes, err := c.getAvailableMonNodes()
	if err != nil {
		return nil, err
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

func (c *Cluster) getAvailableMonNodes() ([]v1.Node, *v1.NodeList, error) {
	nodeOptions := metav1.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	nodes, err := c.context.Clientset.CoreV1().Nodes().List(nodeOptions)
	if err != nil {
		return nil, nil, err
	}
	logger.Debugf("there are %d nodes available for %d mons", len(nodes.Items), len(c.clusterInfo.Monitors))

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

	return availableNodes, nodes, nil
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

func (c *Cluster) startMon(m *monConfig, nodeId string) error {
	rs := c.makeReplicaSet(m, nodeId)
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

func waitForQuorumWithMons(context *clusterd.Context, clusterName string, mons []string) error {
	logger.Infof("waiting for mon quorum")

	// wait for monitors to establish quorum
	retryCount := 0
	retryMax := 20
	sleepTime := 5
	for {
		retryCount++
		if retryCount > retryMax {
			return fmt.Errorf("exceeded max retry count waiting for monitors to reach quorum")
		}

		if retryCount > 1 {
			// only sleep after the first time
			<-time.After(time.Duration(sleepTime) * time.Second)
		}

		// get the mon_status response that contains info about all monitors in the mon map and
		// their quorum status
		monStatusResp, err := client.GetMonStatus(context, clusterName, false)
		if err != nil {
			logger.Debugf("failed to get mon_status, err: %+v", err)
			continue
		}

		// check if each of the initial monitors is in quorum
		allInQuorum := true
		for _, name := range mons {
			// first get the initial monitors corresponding mon map entry
			var monMapEntry *client.MonMapEntry
			for i := range monStatusResp.MonMap.Mons {
				if name == monStatusResp.MonMap.Mons[i].Name {
					monMapEntry = &monStatusResp.MonMap.Mons[i]
					break
				}
			}

			if monMapEntry == nil {
				// found an initial monitor that is not in the mon map, bail out of this retry
				logger.Warningf("failed to find initial monitor %s in mon map", name)
				allInQuorum = false
				break
			}

			// using the current initial monitor's mon map entry, check to see if it's in the quorum list
			// (a list of monitor rank values)
			inQuorumList := false
			for _, q := range monStatusResp.Quorum {
				if monMapEntry.Rank == q {
					inQuorumList = true
					break
				}
			}

			if !inQuorumList {
				// found an initial monitor that is not in quorum, bail out of this retry
				logger.Warningf("initial monitor %s is not in quorum list", name)
				allInQuorum = false
				break
			}
		}

		if allInQuorum {
			logger.Debugf("all initial monitors are in quorum")
			break
		}
	}

	logger.Infof("Ceph monitors formed quorum")
	return nil
}
