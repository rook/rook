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
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// DefaultPort is the default port mons use to communicate with each other
	DefaultPort = 6789
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
	// DataPathMap is the mapping relationship between mon data stored on the host and mon data
	// stored in containers.
	DataPathMap *config.DataPathMap
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
func New(
	context *clusterd.Context,
	namespace, dataDirHostPath, rookVersion string,
	cephVersion cephv1.CephVersionSpec,
	mon cephv1.MonSpec,
	placement rookalpha.Placement,
	hostNetwork bool,
	resources v1.ResourceRequirements,
	ownerRef metav1.OwnerReference,
) *Cluster {
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
	// fail if we were instructed to deploy more than one mon on the same machine with host networking
	if c.HostNetwork && c.AllowMultiplePerNode && c.Count > 1 {
		return fmt.Errorf("refusing to deploy %d monitors on the same host since hostNetwork is %v and allowMultiplePerNode is %v. Only one monitor per node is allowed", c.Count, c.HostNetwork, c.AllowMultiplePerNode)
	}

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

	// Get public IPs (services) for mons all at once so we can build mon_host
	if err := c.initMonIPs(mons); err != nil {
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

	logger.Debugf("mon endpoints are: %s", FlattenMonEndpoints(c.clusterInfo.Monitors))

	// Start (or update) one monitor at a time
	if err := c.startDeployments(mons); err != nil {
		return fmt.Errorf("failed to start mon pods. %+v", err)
	}

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

	// store the keyring which all mons share
	keyring.GetSecretStore(c.context, c.Namespace, &c.ownerRef).
		CreateOrUpdate(keyringStoreName, c.genMonSharedKeyring())

	return nil
}

func (c *Cluster) initMonConfig(size int) []*monConfig {
	mons := []*monConfig{}

	// initialize the mon pod info for mons that have been previously created
	for _, monitor := range c.clusterInfo.Monitors {
		mons = append(mons, &monConfig{
			ResourceName: resourceName(monitor.Name),
			DaemonName:   monitor.Name,
			Port:         getPortFromEndpoint(monitor.Endpoint),
			DataPathMap: config.NewStatefulDaemonDataPathMap(
				c.dataDirHostPath, dataDirRelativeHostPath(monitor.Name), config.MonType, monitor.Name),
		})
	}

	// initialize mon info if we don't have enough mons (at first startup)
	for i := len(c.clusterInfo.Monitors); i < size; i++ {
		c.maxMonID++
		mons = append(mons, c.newMonConfig(c.maxMonID))
	}

	return mons
}

func (c *Cluster) newMonConfig(monID int) *monConfig {
	daemonName := k8sutil.IndexToName(monID)
	return &monConfig{
		ResourceName: resourceName(daemonName),
		DaemonName:   daemonName,
		Port:         int32(DefaultPort),
		DataPathMap: config.NewStatefulDaemonDataPathMap(
			c.dataDirHostPath, dataDirRelativeHostPath(daemonName), config.MonType, daemonName),
	}
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
		c.mapping.Node[m.DaemonName] = nodeInfo
		nodeIndex++
	}

	logger.Debug("mons have been assigned to nodes")
	return nil
}

func (c *Cluster) startDeployments(mons []*monConfig) error {
	if len(mons) == 0 {
		return fmt.Errorf("cannot start 0 mons")
	}

	// mon deployments already created must have a quorum before we can update/add mons
	existingDeployments := 0
	for i, m := range mons {
		if s, err := c.monIsCreated(m); err != nil {
			return fmt.Errorf("could not determine existing mon cluster status. %+v", err)
		} else if !s {
			// because the rook operator creates/updates mons in order, the first non-started mon
			// indicates the end of the preexisting mons in the list
			break
		}
		existingDeployments = i + 1
	}
	if existingDeployments > 0 && c.waitForStart {
		if err := c.waitForQuorumWithMons(mons[0:existingDeployments]); err != nil {
			/* TODO: take steps to heal cluster */
			return fmt.Errorf("preexisting mon cluster cannot establish quorum. %+v", err)
		}
	}

	// when updating, make sure full quorum is maintained for each individual updated daemon
	for i := 0; i < existingDeployments; i++ {
		node, _ := c.mapping.Node[mons[i].DaemonName]
		if err := c.updateMon(mons[i], node.Hostname); err != nil {
			return fmt.Errorf("failed to update mon daemon %s. %+v", mons[i].DaemonName, err)
		}
		if c.waitForStart {
			if err := c.waitForQuorumWithMons(mons[0:existingDeployments]); err != nil {
				/* TODO: take steps to heal cluster */
				return fmt.Errorf("mon cluster failed to establish quorum. %+v", err)
			}
		}
	}

	// when creating mons, we just want to create all of them and then wait for them to join quorum
	for i := existingDeployments; i < len(mons); i++ {
		node, _ := c.mapping.Node[mons[i].DaemonName]
		if err := c.createMon(mons[i], node.Hostname); err != nil {
			return fmt.Errorf("failed to start mon %s. %+v", mons[i].DaemonName, err)
		}
	}
	if c.waitForStart {
		if err := c.waitForQuorumWithMons(mons); err != nil {
			/* TODO: take steps to heal cluster */
			return fmt.Errorf("mon cluster failed to establish quorum. %+v", err)
		}
	}

	logger.Infof("%d mons updated, %d mons created", existingDeployments, len(mons)-existingDeployments)
	return nil
}

func (c *Cluster) monIsCreated(m *monConfig) (bool, error) {
	_, err := c.context.Clientset.Extensions().Deployments(c.Namespace).Get(m.ResourceName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *Cluster) createMon(m *monConfig, hostname string) error {
	d := c.makeDeployment(m, hostname)
	logger.Debugf("Creating mon: %+v", d.Name)
	if _, err := c.context.Clientset.Extensions().Deployments(c.Namespace).Create(d); err != nil {
		return fmt.Errorf("failed to create mon %s. %+v", m.ResourceName, err)
	}
	return nil
}

var updateDeploymentAndWait = k8sutil.UpdateDeploymentAndWait

func (c *Cluster) updateMon(m *monConfig, hostname string) error {
	d := c.makeDeployment(m, hostname)
	logger.Debugf("Updating mon: %+v", d.Name)

	if err := updateDeploymentAndWait(c.context, d, c.Namespace); err != nil {
		return fmt.Errorf("failed to update mon deployment %s. %+v", m.ResourceName, err)
	}
	return nil
}

func (c *Cluster) waitForQuorumWithMons(mons []*monConfig) error {
	monIDs := []string{}
	for _, m := range mons {
		monIDs = append(monIDs, m.DaemonName)
	}
	logger.Infof("waiting for mon quorum with %v", monIDs)

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
		running, err := k8sutil.PodsRunningWithLabel(c.context.Clientset, c.Namespace, "app="+appName)
		if err != nil {
			logger.Warningf("failed to query mon pod status, trying again. %+v", err)
			continue
		}
		if running != len(monIDs) {
			logger.Infof("%d/%d mon pods are running. waiting for pods to start", running, len(monIDs))
			continue
		}

		// get mon_status response w/ info about all monitors in the mon map and their quorum status
		status, err := client.GetMonStatus(c.context, c.Namespace, false)
		if err != nil {
			logger.Debugf("failed to get mon_status, err: %+v", err)
			continue
		}

		// check if each of the initial monitors is in quorum
		allInQuorum := true
		for _, id := range monIDs {
			if !monIsInQuorum(id, status) {
				allInQuorum = false
				break
			}
		}

		if allInQuorum {
			break
		}
	}

	logger.Infof("Ceph monitors formed quorum")
	return nil
}

func monIsInQuorum(monID string, status client.MonStatusResponse) bool {
	var mon *client.MonMapEntry
	for i := range status.MonMap.Mons {
		if monID == status.MonMap.Mons[i].Name {
			mon = &status.MonMap.Mons[i]
			break
		}
	}

	if mon == nil {
		logger.Warningf("failed to find monitor %s in Ceph mon map", monID)
		return false
	}

	for _, q := range status.Quorum {
		if mon.Rank == q {
			return true
		}
	}

	logger.Warningf("monitor %s is not in quorum list", monID)
	return false
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
		EndpointDataKey: FlattenMonEndpoints(c.clusterInfo.Monitors),
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

	// Every time the mon config is updated, must also update the global config so that all daemons
	// have the most updated version if they restart.
	config.GetStore(c.context, c.Namespace, &c.ownerRef).CreateOrUpdate(c.clusterInfo)

	// write the latest config to the config dir
	if err := writeConnectionConfig(c.context, c.clusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config for new mons. %+v", err)
	}

	return nil
}
