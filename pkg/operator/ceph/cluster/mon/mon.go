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

// Package mon provides methods for creating clusters of Ceph mons in Kubernetes, for monitoring the
// cluster's status, for taking corrective actions if the status is non-ideal, and for reporting
// mon cluster failures.
package mon

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	mondaemon "github.com/rook/rook/pkg/daemon/ceph/mon"
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

	// DefaultMonCount Default mon count for a cluster
	DefaultMonCount = 3
	// MaxMonCount Maximum allowed mon count for a cluster
	MaxMonCount = 9
)

// Cluster represents the Rook and environment configuration settings needed to set up Ceph mons.
type Cluster struct {
	context              *clusterd.Context
	Namespace            string
	Keyring              string
	rookVersion          string
	cephVersion          cephv1.CephVersionSpec
	Count                int
	AllowMultiplePerNode bool
	MonCountMutex        sync.Mutex
	Port                 int32
	clusterInfo          *cephconfig.ClusterInfo
	placement            rookalpha.Placement
	maxMonID             int
	waitForStart         bool
	dataDirHostPath      string
	monPodRetryInterval  time.Duration
	monPodTimeout        time.Duration
	monTimeoutList       map[string]time.Time
	HostNetwork          bool
	mapping              *Mapping
	resources            v1.ResourceRequirements
	ownerRef             metav1.OwnerReference
}

// monConfig for a single monitor
type monConfig struct {
	// ResourceName is the name given to the mon's Kubernetes resources in metadata
	ResourceName string
	// DaemonName is the name given the mon daemon ("a", "b", "c,", etc.)
	DaemonName string
	// PublicIP is the IP of the mon's service that the mon will receive connections on
	PublicIP string
	// Port is the port on which the mon will listen for connections
	Port int32
}

// Mapping is mon node and port mapping
type Mapping struct {
	Node map[string]*NodeInfo `json:"node"`
	Port map[string]int32     `json:"port"`
}

// NodeInfo contains name and address of a node
type NodeInfo struct {
	Name     string
	Hostname string
	Address  string
}

// New creates an instance of a mon cluster
func New(context *clusterd.Context, namespace, dataDirHostPath, rookVersion string, cephVersion cephv1.CephVersionSpec, mon cephv1.MonSpec,
	placement rookalpha.Placement, hostNetwork bool, resources v1.ResourceRequirements, ownerRef metav1.OwnerReference) *Cluster {
	return &Cluster{
		context:              context,
		placement:            placement,
		dataDirHostPath:      dataDirHostPath,
		Namespace:            namespace,
		rookVersion:          rookVersion,
		cephVersion:          cephVersion,
		Count:                mon.Count,
		AllowMultiplePerNode: mon.AllowMultiplePerNode,
		maxMonID:             -1,
		waitForStart:         true,
		monPodRetryInterval:  6 * time.Second,
		monPodTimeout:        5 * time.Minute,
		monTimeoutList:       map[string]time.Time{},
		HostNetwork:          hostNetwork,
		mapping: &Mapping{
			Node: map[string]*NodeInfo{},
			Port: map[string]int32{},
		},
		resources: resources,
		ownerRef:  ownerRef,
	}
}

// Start begins the process of running a cluster of Ceph mons.
func (c *Cluster) Start() error {
	logger.Infof("start running mons")

	if err := c.initClusterInfo(); err != nil {
		return fmt.Errorf("failed to initialize ceph cluster info. %+v", err)
	}

	// create the mons for a new cluster or ensure mons are running in an existing cluster
	return c.startMons()
}

func (c *Cluster) startMons() error {
	// init the mons config
	mons := c.initMonConfig(c.Count)

	// Assign the pods to nodes
	if err := c.assignMons(mons); err != nil {
		return fmt.Errorf("failed to assign pods to mons. %+v", err)
	}

	// Start one monitor at a time
	for i := 0; i < c.Count; i++ {
		logger.Infof("ensuring mon %s (%s) is started", mons[i].ResourceName, mons[i].DaemonName)
		endIndex := len(c.clusterInfo.Monitors)
		if endIndex < c.Count {
			endIndex++
		}
		logger.Infof("looping to start mons. i=%d, endIndex=%d, c.Size=%d", i, endIndex, c.Count)

		// Init the mon IPs
		if err := c.initMonIPs(mons[0:endIndex]); err != nil {
			return fmt.Errorf("failed to init mon services. %+v", err)
		}

		// save the mon config after we have "initiated the IPs"
		if err := c.saveMonConfig(); err != nil {
			return fmt.Errorf("failed to save mons. %+v", err)
		}

		// make sure we have the connection info generated so connections can happen
		if err := writeConnectionConfig(c.context, c.clusterInfo); err != nil {
			return err
		}

		// Start the deployment
		if err := c.startDeployments(mons[0:endIndex], i); err != nil {
			return fmt.Errorf("failed to start mon pods. %+v", err)
		}
	}

	logger.Debugf("mon endpoints used are: %s", mondaemon.FlattenMonEndpoints(c.clusterInfo.Monitors))
	return nil
}

// initClusterInfo retrieves the ceph cluster info if it already exists.
// If a new cluster, create new keys.
func (c *Cluster) initClusterInfo() error {
	var err error
	// get the cluster info from secret
	c.clusterInfo, c.maxMonID, c.mapping, err = CreateOrLoadClusterInfo(c.context, c.Namespace, &c.ownerRef)
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
		mons = append(mons, &monConfig{ResourceName: resourceName(monitor.Name), DaemonName: monitor.Name, Port: getPortFromEndpoint(monitor.Endpoint)})
	}

	// initialize mon info if we don't have enough mons (at first startup)
	for i := len(c.clusterInfo.Monitors); i < size; i++ {
		c.maxMonID++
		mons = append(mons, newMonConfig(c.maxMonID))
	}

	return mons
}

func newMonConfig(monID int) *monConfig {
	daemonName := k8sutil.IndexToName(monID)
	return &monConfig{ResourceName: resourceName(daemonName), DaemonName: daemonName, Port: int32(mondaemon.DefaultPort)}
}

// resourceName ensures the mon name has the rook-ceph-mon prefix
func resourceName(name string) string {
	if strings.HasPrefix(name, appName) {
		return name
	}
	return fmt.Sprintf("%s-%s", appName, name)
}

func (c *Cluster) initMonIPs(mons []*monConfig) error {
	for _, m := range mons {
		if c.HostNetwork {
			logger.Infof("setting mon endpoints for hostnetwork mode")
			node, ok := c.mapping.Node[m.DaemonName]
			if !ok {
				return fmt.Errorf("mon doesn't exist in assignment map")
			}
			m.PublicIP = node.Address
		} else {
			serviceIP, err := c.createService(m)
			if err != nil {
				return fmt.Errorf("failed to create mon service. %+v", err)
			}
			m.PublicIP = serviceIP
		}
		c.clusterInfo.Monitors[m.DaemonName] = cephconfig.NewMonInfo(m.DaemonName, m.PublicIP, m.Port)
	}

	return nil
}

func (c *Cluster) createService(mon *monConfig) (string, error) {
	labels := c.getLabels(mon.DaemonName)
	s := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   mon.ResourceName,
			Labels: labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       mon.ResourceName,
					Port:       mon.Port,
					TargetPort: intstr.FromInt(int(mon.Port)),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &s.ObjectMeta, &c.ownerRef)
	if c.HostNetwork {
		s.Spec.ClusterIP = v1.ClusterIPNone
	}

	s, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(s)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return "", fmt.Errorf("failed to create mon service. %+v", err)
		}
		s, err = c.context.Clientset.CoreV1().Services(c.Namespace).Get(mon.ResourceName, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to get mon %s service ip. %+v", mon.ResourceName, err)
		}
	}

	if s == nil {
		logger.Warningf("service ip not found for mon %s. this better be a test", mon.ResourceName)
		return "", nil
	}

	logger.Infof("mon %s running at %s:%d", mon.DaemonName, s.Spec.ClusterIP, mon.Port)
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
		if _, ok := c.mapping.Node[m.DaemonName]; ok {
			logger.Debugf("mon %s already assigned to a node, no need to assign", m.DaemonName)
			continue
		}

		// if we need to place a new mon and don't have any more nodes available, we fail to add the mon
		if len(availableNodes) == 0 {
			return fmt.Errorf("no nodes available for mon placement")
		}

		// pick one of the available nodes where the mon will be assigned
		node := availableNodes[nodeIndex%len(availableNodes)]
		logger.Debugf("mon %s assigned to node %s", m.DaemonName, node.Name)
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
		c.mapping.Node[m.DaemonName] = nodeInfo
		nodeIndex++
	}

	logger.Debug("mons have been assigned to nodes")
	return nil
}

func getNodeInfoFromNode(n v1.Node) (*NodeInfo, error) {
	nr := &NodeInfo{
		Name:     n.Name,
		Hostname: n.Labels[apis.LabelHostname],
	}

	for _, ip := range n.Status.Addresses {
		if ip.Type == v1.NodeExternalIP || ip.Type == v1.NodeInternalIP {
			logger.Debugf("using IP %s for node %s", ip.Address, n.Name)
			nr.Address = ip.Address
			break
		}
	}
	if nr.Address == "" {
		return nil, fmt.Errorf("couldn't get IP of node %s", nr.Name)
	}
	return nr, nil
}

func (c *Cluster) startDeployments(mons []*monConfig, index int) error {
	if len(mons) == 0 {
		return fmt.Errorf("cannot start 0 mons")
	}

	// Ensure each of the mons have been created. If already created, it will be a no-op.
	for i := 0; i < len(mons); i++ {
		node, _ := c.mapping.Node[mons[i].DaemonName]
		err := c.startMon(mons[i], node.Hostname)
		if err != nil {
			return fmt.Errorf("failed to create mon %s. %+v", mons[i].DaemonName, err)
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
		starting = append(starting, m.DaemonName)
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
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &configMap.ObjectMeta, &c.ownerRef)

	monMapping, err := json.Marshal(c.mapping)
	if err != nil {
		return fmt.Errorf("failed to marshal mon mapping. %+v", err)
	}

	configMap.Data = map[string]string{
		EndpointDataKey: mondaemon.FlattenMonEndpoints(c.clusterInfo.Monitors),
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
	if err := writeConnectionConfig(c.context, c.clusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config for new mons. %+v", err)
	}

	return nil
}

// getMonNodes detects the nodes that are available for new mons to start.
func (c *Cluster) getMonNodes() ([]v1.Node, error) {
	availableNodes, nodes, err := c.getAvailableMonNodes()
	if err != nil {
		return nil, err
	}
	logger.Infof("Found %d running nodes without mons", len(availableNodes))

	// if all nodes already have mons and the user has given the mon.count, add all nodes to be available
	if c.AllowMultiplePerNode && len(availableNodes) == 0 {
		logger.Infof("All nodes are running mons. Adding all %d nodes to the availability.", len(nodes.Items))
		for _, node := range nodes.Items {
			valid, err := k8sutil.ValidNode(node, c.placement)
			if err != nil {
				logger.Warning("failed to validate node %s %v", node.Name, err)
			} else if valid {
				availableNodes = append(availableNodes, node)
			}
		}
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
	nodesInUse, err := c.getNodesWithMons(nodes)
	if err != nil {
		logger.Warningf("could not get nodes with mons. %+v", err)
		nodesInUse = util.NewSet()
	}

	// choose nodes for the new mons that don't have mons currently
	availableNodes := []v1.Node{}
	for _, node := range nodes.Items {
		if !nodesInUse.Contains(node.Name) {
			valid, err := k8sutil.ValidNode(node, c.placement)
			if err != nil {
				logger.Warning("failed to validate node %s %v", node.Name, err)
			} else if valid {
				availableNodes = append(availableNodes, node)
			}
		}
	}

	return availableNodes, nodes, nil
}

func (c *Cluster) getNodesWithMons(nodes *v1.NodeList) (*util.Set, error) {
	// get the mon pods and their node affinity
	options := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", appName)}
	pods, err := c.context.Clientset.CoreV1().Pods(c.Namespace).List(options)
	if err != nil {
		return nil, err
	}
	nodesInUse := util.NewSet()
	for _, pod := range pods.Items {
		hostname := pod.Spec.NodeSelector[apis.LabelHostname]
		logger.Debugf("mon pod on node %s", hostname)
		name, ok := getNodeNameFromHostname(nodes, hostname)
		if !ok {
			logger.Errorf("mon %s on hostname %s not found in node list", pod.Name, hostname)
		}
		nodesInUse.Add(name)
	}
	return nodesInUse, nil
}

// Look up the immutable node name from the hostname label
func getNodeNameFromHostname(nodes *v1.NodeList, hostname string) (string, bool) {
	for _, node := range nodes.Items {
		if node.Labels[apis.LabelHostname] == hostname {
			return node.Name, true
		}
		if node.Name == hostname {
			return node.Name, true
		}
	}
	return "", false
}

var updateDeploymentAndWait = k8sutil.UpdateDeploymentAndWait

func (c *Cluster) startMon(m *monConfig, hostname string) error {
	// If we determine the legacy replicaset exists, delete it so we can start the new deployment in its place
	if err := k8sutil.DeleteReplicaSet(c.context.Clientset, c.Namespace, m.ResourceName); err != nil {
		logger.Errorf("failed to delete legacy mon replicaset. %+v", err)
	}

	d := c.makeDeployment(m, hostname)
	logger.Debugf("Starting mon: %+v", d.Name)
	_, err := c.context.Clientset.Extensions().Deployments(c.Namespace).Create(d)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create mon %s. %+v", m.ResourceName, err)
		}
		logger.Debugf("deployment for mon %s already exists. updating if needed", m.ResourceName)
		p, err := c.context.Clientset.Extensions().Deployments(c.Namespace).Get(d.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to update mon deployment %s. failed to inspect preexisting deployment. %+v", d.Name, err)
		}
		// Workaround for #2331 targeted for Rook v0.9: only update the deployment if the Rook init
		// image or Ceph image has changed.
		if p.Spec.Template.Spec.Containers[0].Image != d.Spec.Template.Spec.Containers[0].Image ||
			p.Spec.Template.Spec.InitContainers[0].Image != d.Spec.Template.Spec.InitContainers[0].Image {
			if err := updateDeploymentAndWait(c.context, d, c.Namespace); err != nil {
				return fmt.Errorf("failed to update mon deployment %s. %+v", m.ResourceName, err)
			}
		}
	}

	return nil
}

func waitForQuorumWithMons(context *clusterd.Context, clusterName string, mons []string) error {
	logger.Infof("waiting for mon quorum with %v", mons)

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

		// wait for the mon pods to be running
		running, err := k8sutil.PodsRunningWithLabel(context.Clientset, clusterName, "app="+appName)
		if err != nil {
			logger.Infof("failed to query mon pod status, trying again. %+v", err)
			continue
		}
		if running != len(mons) {
			logger.Infof("%d/%d mon pods are running. waiting for pods to start", running, len(mons))
			continue
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
