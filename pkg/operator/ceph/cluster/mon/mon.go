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
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

	// configuration map for csi
	csiConfigKey = "csi-cluster-config-json"

	// DefaultMonCount Default mon count for a cluster
	DefaultMonCount = 3
	// MaxMonCount Maximum allowed mon count for a cluster
	MaxMonCount = 9

	// DefaultMsgr1Port is the default port Ceph mons use to communicate amongst themselves prior
	// to Ceph Nautilus.
	DefaultMsgr1Port int32 = 6789
	// DefaultMsgr2Port is the listening port of the messenger v2 protocol introduced in Ceph
	// Nautilus. In Nautilus and a few Ceph releases after, Ceph can use both v1 and v2 protocols.
	DefaultMsgr2Port int32 = 3300

	// minimum amount of memory in MB to run the pod
	cephMonPodMinimumMemory uint64 = 1024
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mon")
)

// Cluster represents the Rook and environment configuration settings needed to set up Ceph mons.
type Cluster struct {
	clusterInfo         *cephconfig.ClusterInfo
	context             *clusterd.Context
	spec                cephv1.ClusterSpec
	Namespace           string
	Keyring             string
	rookVersion         string
	orchestrationMutex  sync.Mutex
	Port                int32
	HostNetwork         bool
	maxMonID            int
	waitForStart        bool
	dataDirHostPath     string
	monPodRetryInterval time.Duration
	monPodTimeout       time.Duration
	monTimeoutList      map[string]time.Time
	mapping             *Mapping
	ownerRef            metav1.OwnerReference
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
func New(context *clusterd.Context, namespace, dataDirHostPath string, hostNetwork bool, ownerRef metav1.OwnerReference) *Cluster {
	return &Cluster{
		context:             context,
		dataDirHostPath:     dataDirHostPath,
		Namespace:           namespace,
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
		ownerRef: ownerRef,
	}
}

// Start begins the process of running a cluster of Ceph mons.
func (c *Cluster) Start(clusterInfo *cephconfig.ClusterInfo, rookVersion string, cephVersion cephver.CephVersion, spec cephv1.ClusterSpec) (*cephconfig.ClusterInfo, error) {

	// Only one goroutine can orchestrate the mons at a time
	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.clusterInfo = clusterInfo
	c.rookVersion = rookVersion
	c.spec = spec

	// fail if we were instructed to deploy more than one mon on the same machine with host networking
	if c.HostNetwork && c.spec.Mon.AllowMultiplePerNode && c.spec.Mon.Count > 1 {
		return nil, fmt.Errorf("refusing to deploy %d monitors on the same host since hostNetwork is %v and allowMultiplePerNode is %v. Only one monitor per node is allowed", c.spec.Mon.Count, c.HostNetwork, c.spec.Mon.AllowMultiplePerNode)
	}

	// Validate pod's memory if specified
	err := opspec.CheckPodMemory(cephv1.GetMonResources(c.spec.Resources), cephMonPodMinimumMemory)
	if err != nil {
		return nil, fmt.Errorf("%v", err)
	}

	logger.Infof("start running mons")

	logger.Debugf("establishing ceph cluster info")
	if err := c.initClusterInfo(cephVersion); err != nil {
		return nil, fmt.Errorf("failed to initialize ceph cluster info. %+v", err)
	}

	targetCount, msg, err := c.getTargetMonCount()
	if err != nil {
		return nil, fmt.Errorf("failed to get target mon count. %+v", err)
	}
	logger.Infof(msg)

	// create the mons for a new cluster or ensure mons are running in an existing cluster
	return c.clusterInfo, c.startMons(targetCount)
}

func (c *Cluster) startMons(targetCount int) error {
	// init the mon config
	existingCount, mons := c.initMonConfig(targetCount)

	// Assign the mons to nodes
	if err := c.assignMons(mons); err != nil {
		return fmt.Errorf("failed to assign pods to mons. %+v", err)
	}

	if existingCount < len(mons) {
		// Start the new mons one at a time
		for i := existingCount; i < targetCount; i++ {
			if err := c.ensureMonsRunning(mons, i, targetCount, true); err != nil {
				return err
			}
		}
	} else {
		// Ensure all the expected mon deployments exist, but don't require full quorum to continue
		lastMonIndex := len(mons) - 1
		if err := c.ensureMonsRunning(mons, lastMonIndex, targetCount, false); err != nil {
			return err
		}
	}

	// Enable Ceph messenger 2 protocol on Nautilus
	if c.clusterInfo.CephVersion.IsAtLeastNautilus() {
		v, err := client.GetCephMonVersion(c.context, c.clusterInfo.Name)
		if err != nil {
			return fmt.Errorf("failed to get ceph mon version. %+v", err)
		}
		if v.IsAtLeastNautilus() {
			versions, err := client.GetAllCephDaemonVersions(c.context, c.clusterInfo.Name)
			if err != nil {
				return fmt.Errorf("failed to get ceph daemons versions. %+v", err)
			}
			if len(versions.Mon) == 1 {
				// If length is one, this clearly indicates that all the mons are running the same version
				// We are doing this because 'ceph version' might return the Ceph version that a majority of mons has but not all of them
				// so instead of trying to active msgr2 when mons are not ready, we activate it when we believe that's the right time
				client.EnableMessenger2(c.context)
			}
		}
	}

	logger.Debugf("mon endpoints used are: %s", FlattenMonEndpoints(c.clusterInfo.Monitors))
	return nil
}

// ensureMonsRunning is called in two scenarios:
// 1. To create a new mon and wait for it to join quorum (requireAllInQuorum = true). This method will be called multiple times
//    to add a mon until we have reached the desired number of mons.
// 2. To check that the majority of existing mons are in quorum. It is ok if not all mons are in quorum. (requireAllInQuorum = false)
//    This is needed when the operator is restarted and all mons may not be up or in quorum.
func (c *Cluster) ensureMonsRunning(mons []*monConfig, i, targetCount int, requireAllInQuorum bool) error {
	if requireAllInQuorum {
		logger.Infof("creating mon %s", mons[i].DaemonName)
	} else {
		logger.Info("checking for basic quorum with existing mons")
	}

	// Calculate how many mons we expected to exist after this method is completed.
	// If we are adding a new mon, we expect one more than currently exist.
	// If we haven't created all the desired mons already, we will be adding a new one with this iteration
	expectedMonCount := len(c.clusterInfo.Monitors)
	if expectedMonCount < targetCount {
		expectedMonCount++
	}

	// Init the mon IPs
	if err := c.initMonIPs(mons[0:expectedMonCount]); err != nil {
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

	// Start the deployment
	if err := c.startDeployments(mons[0:expectedMonCount], requireAllInQuorum); err != nil {
		return fmt.Errorf("failed to start mon pods. %+v", err)
	}

	return nil
}

// initClusterInfo retrieves the ceph cluster info if it already exists.
// If a new cluster, create new keys.
func (c *Cluster) initClusterInfo(cephVersion cephver.CephVersion) error {
	var err error

	// get the cluster info from secret
	c.clusterInfo, c.maxMonID, c.mapping, err = CreateOrLoadClusterInfo(c.context, c.Namespace, &c.ownerRef)
	if err != nil {
		return fmt.Errorf("failed to get cluster info. %+v", err)
	}
	c.clusterInfo.CephVersion = cephVersion

	// save cluster monitor config
	if err = c.saveMonConfig(); err != nil {
		return fmt.Errorf("failed to save mons. %+v", err)
	}

	k := keyring.GetSecretStore(c.context, c.Namespace, &c.ownerRef)
	// store the keyring which all mons share
	if err := k.CreateOrUpdate(keyringStoreName, c.genMonSharedKeyring()); err != nil {
		return fmt.Errorf("failed to save mon keyring secret. %+v", err)
	}
	// also store the admin keyring for other daemons that might need it during init
	if err := k.Admin().CreateOrUpdate(c.clusterInfo); err != nil {
		return fmt.Errorf("failed to save admin keyring secret. %+v", err)
	}

	return nil
}

func (c *Cluster) initMonConfig(size int) (int, []*monConfig) {
	mons := []*monConfig{}

	// initialize the mon pod info for mons that have been previously created
	for _, monitor := range c.clusterInfo.Monitors {
		mons = append(mons, &monConfig{
			ResourceName: resourceName(monitor.Name),
			DaemonName:   monitor.Name,
			Port:         cephutil.GetPortFromEndpoint(monitor.Endpoint),
			DataPathMap: config.NewStatefulDaemonDataPathMap(
				c.dataDirHostPath, dataDirRelativeHostPath(monitor.Name), config.MonType, monitor.Name, c.Namespace),
		})
	}

	// initialize mon info if we don't have enough mons (at first startup)
	existingCount := len(c.clusterInfo.Monitors)
	for i := len(c.clusterInfo.Monitors); i < size; i++ {
		c.maxMonID++
		mons = append(mons, c.newMonConfig(c.maxMonID))
	}

	return existingCount, mons
}

func (c *Cluster) newMonConfig(monID int) *monConfig {
	daemonName := k8sutil.IndexToName(monID)

	return &monConfig{
		ResourceName: resourceName(daemonName),
		DaemonName:   daemonName,
		Port:         DefaultMsgr1Port,
		DataPathMap: config.NewStatefulDaemonDataPathMap(
			c.dataDirHostPath, dataDirRelativeHostPath(daemonName), config.MonType, daemonName, c.Namespace),
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

// find a suitable node for this monitor. first look for a fault zone that has
// no monitors. only after the nodes with zone labels are considered are the
// unlabeled nodes considered. reasonsing: without more information, _at worst_
// the unlabled nodes are actually in the set of labeled zones, and at best,
// they are in distinct zones.
func scheduleMonitor(mon *monConfig, nodeZones [][]NodeUsage) *NodeUsage {
	// the node choice for this monitor
	var nodeChoice *NodeUsage

	// for each zone, in order of preference. unlabeled nodes are last, by
	// construction; see Cluster.getNodeMonusage().
	for zi := range nodeZones {
		// number of monitors in the zone
		zoneMonCount := 0

		// preferential node choice from this zone for a new mon
		var zoneNodeChoice *NodeUsage

		// for each node in this zone
		for ni := range nodeZones[zi] {
			nodeUsage := &nodeZones[zi][ni]

			// skip invalid nodes. but update the mon count first since
			// invalid nodes could still have an assigned mon pod.
			zoneMonCount += nodeUsage.MonCount
			if !nodeUsage.MonValid {
				logger.Infof("schedmon: skip invalid node %s for mon scheduling",
					nodeUsage.Node.Name)
				continue
			}

			// make a "best" choice from this zone. in this case that is the
			// node with the least amount of monitors.
			if zoneNodeChoice == nil || nodeUsage.MonCount < zoneNodeChoice.MonCount {
				logger.Infof("schedmon: considering node %s with mon count %d",
					nodeUsage.Node.Name, nodeUsage.MonCount)
				zoneNodeChoice = nodeUsage
			}
		}

		// We identified _a_ valid node in the zone. should we choose it?
		if zoneNodeChoice != nil {
			if zoneMonCount == 0 {
				// this zone has no monitors, which implies that
				// zoneNodeChoice will be a node without any monitors
				// currently assigned. choose this node.
				logger.Infof("schedmon: considering node %s from empty zone",
					zoneNodeChoice.Node.Name)
				nodeChoice = zoneNodeChoice
				break
			} else {
				// this zone already has a monitor in it. but keep the
				// choice as a backup; we may find that all zones already
				// have a monitor and still need to make an assignment.
				if nodeChoice == nil || zoneNodeChoice.MonCount < nodeChoice.MonCount {
					logger.Infof("schedmon: considering node %s with mon count %d",
						zoneNodeChoice.Node.Name, zoneNodeChoice.MonCount)
					nodeChoice = zoneNodeChoice
				}
			}
		}
	}

	if nodeChoice != nil {
		logger.Infof("schedmon: scheduling mon %s on node %s",
			mon.DaemonName, nodeChoice.Node.Name)
	} else {
		logger.Infof("schedmon: no suitable node found for mon %s",
			mon.DaemonName)
	}

	return nodeChoice
}

func (c *Cluster) assignMons(mons []*monConfig) error {

	// retrieve the set of cluster nodes and their monitor usage info
	nodeZones, err := c.getNodeMonUsage()
	if err != nil {
		return fmt.Errorf("failed to get node monitor usage. %+v", err)
	}

	// ensure all monitors have a node assignment. note that this isn't
	// necessarily optimal: it does not try to move existing monitors which is
	// handled by the periodic monitor health checks.
	for _, mon := range mons {

		// monitor is already assigned to a node. nothing to do
		if _, ok := c.mapping.Node[mon.DaemonName]; ok {
			logger.Infof("assignmon: mon %s already assigned to a node", mon.DaemonName)
			continue
		}

		nodeChoice := scheduleMonitor(mon, nodeZones)

		if nodeChoice == nil {
			return fmt.Errorf("assignmon: no valid nodes available for mon placement")
		}

		// note that we do not need to worry about a false negative here (i.e. a
		// node exists that _would_ pass this check) due to the search landing in a
		// local minima because if a node had existed with a mon count of zero,
		// scheduleMonitor would have chosen it over any node with a positive
		// mon count.
		if nodeChoice.MonCount > 0 && !c.spec.Mon.AllowMultiplePerNode {
			return fmt.Errorf("assignmon: no empty nodes available for mon placement")
		}

		// make this decision visible when scheduling the next monitor
		nodeChoice.MonCount++

		logger.Infof("assignmon: mon %s assigned to node %s", mon.DaemonName, nodeChoice.Node.Name)

		nodeInfo, err := getNodeInfoFromNode(*nodeChoice.Node)
		if err != nil {
			return fmt.Errorf("couldn't get node info from node %s. %+v",
				nodeChoice.Node.Name, err)
		}

		c.mapping.Node[mon.DaemonName] = nodeInfo
	}

	logger.Debug("assignmons: mons have been assigned to nodes")
	return nil
}

func (c *Cluster) startDeployments(mons []*monConfig, requireAllInQuorum bool) error {
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
		// For the initial deployment (first creation) it's expected to not have all the monitors in quorum
		// However, in an event of an update, it's crucial to proceed monitors by monitors
		// At the end of the method we perform one last check where all the monitors must be in quorum
		requireAllInQuorum := false
		err = c.waitForMonsToJoin(mons, requireAllInQuorum)
		if err != nil {
			return fmt.Errorf("failed to check mon quorum %s. %+v", mons[i].DaemonName, err)
		}
	}

	logger.Infof("mons created: %d", len(mons))
	// Final verification that **all** mons are in quorum
	// Do not proceed if one monitor is still syncing
	// Only do this when monitors versions are different so we don't block the orchestration if a mon is down.
	versions, err := client.GetAllCephDaemonVersions(c.context, c.clusterInfo.Name)
	if err != nil {
		logger.Warningf("failed to get ceph daemons versions; this likely means there is no cluster yet. %+v", err)
	} else {
		if len(versions.Mon) != 1 {
			requireAllInQuorum = true
		}
	}
	return c.waitForMonsToJoin(mons, requireAllInQuorum)
}

func (c *Cluster) waitForMonsToJoin(mons []*monConfig, requireAllInQuorum bool) error {
	if !c.waitForStart {
		return nil
	}

	starting := []string{}
	for _, m := range mons {
		starting = append(starting, m.DaemonName)
	}

	// wait for the monitors to join quorum
	sleepTime := 5
	err := waitForQuorumWithMons(c.context, c.clusterInfo.Name, starting, sleepTime, requireAllInQuorum)
	if err != nil {
		return fmt.Errorf("failed to wait for mon quorum. %+v", err)
	}

	return nil
}

func (c *Cluster) saveMonConfig() error {
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      EndpointConfigMapName,
			Namespace: c.Namespace,
		},
	}
	k8sutil.SetOwnerRef(&configMap.ObjectMeta, &c.ownerRef)

	monMapping, err := json.Marshal(c.mapping)
	if err != nil {
		return fmt.Errorf("failed to marshal mon mapping. %+v", err)
	}

	csiConfigValue, err := FormatCsiClusterConfig(
		c.Namespace, c.clusterInfo.Monitors)
	if err != nil {
		return fmt.Errorf("failed to format csi config: %+v", err)
	}

	configMap.Data = map[string]string{
		EndpointDataKey: FlattenMonEndpoints(c.clusterInfo.Monitors),
		MaxMonIDKey:     strconv.Itoa(c.maxMonID),
		MappingKey:      string(monMapping),
		csiConfigKey:    csiConfigValue,
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
	if err := WriteConnectionConfig(c.context, c.clusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config for new mons. %+v", err)
	}

	return nil
}

var updateDeploymentAndWait = UpdateCephDeploymentAndWait

func (c *Cluster) startMon(m *monConfig, hostname string) error {

	d := c.makeDeployment(m, hostname)
	logger.Debugf("Starting mon: %+v", d.Name)
	_, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Create(d)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create mon deployment %s. %+v", m.ResourceName, err)
		}
		logger.Infof("deployment for mon %s already exists. updating if needed", m.ResourceName)

		// Always invoke ceph version before an upgrade so we are sure to be up-to-date
		daemonType := string(config.MonType)
		var cephVersionToUse cephver.CephVersion
		currentCephVersion, err := client.LeastUptodateDaemonVersion(c.context, c.clusterInfo.Name, daemonType)
		if err != nil {
			logger.Warningf("failed to retrieve current ceph %s version. %+v", daemonType, err)
			logger.Debug("could not detect ceph version during update, this is likely an initial bootstrap, proceeding with c.clusterInfo.CephVersion")
			cephVersionToUse = c.clusterInfo.CephVersion
		} else {
			logger.Debugf("current cluster version for monitors before upgrading is: %+v", currentCephVersion)
			cephVersionToUse = currentCephVersion
		}
		if err := updateDeploymentAndWait(c.context, d, c.Namespace, daemonType, m.DaemonName, cephVersionToUse); err != nil {
			return fmt.Errorf("failed to update mon deployment %s. %+v", m.ResourceName, err)
		}
	}

	return nil
}

func waitForQuorumWithMons(context *clusterd.Context, clusterName string, mons []string, sleepTime int, requireAllInQuorum bool) error {
	logger.Infof("waiting for mon quorum with %v", mons)

	// wait for monitors to establish quorum
	retryCount := 0
	retryMax := 30
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
		allPodsRunning := true
		var runningMonNames []string
		for _, m := range mons {
			running, err := k8sutil.PodsRunningWithLabel(context.Clientset, clusterName, fmt.Sprintf("app=%s,mon=%s", appName, m))
			if err != nil {
				logger.Infof("failed to query mon pod status, trying again. %+v", err)
				continue
			}
			if running > 0 {
				runningMonNames = append(runningMonNames, m)
			} else {
				allPodsRunning = false
				logger.Infof("mon %s is not yet running", m)
			}
		}

		logger.Infof("mons running: %v", runningMonNames)
		if !allPodsRunning && requireAllInQuorum {
			continue
		}

		// get the mon_status response that contains info about all monitors in the mon map and
		// their quorum status
		monStatusResp, err := client.GetMonStatus(context, clusterName, false)
		if err != nil {
			logger.Debugf("failed to get mon_status, err: %+v", err)
			continue
		}

		if !requireAllInQuorum {
			logQuorumMembers(monStatusResp)
			break
		}

		// check if each of the initial monitors is in quorum
		allInQuorum := true
		for _, name := range mons {
			if !monFoundInQuorum(name, monStatusResp) {
				// found an initial monitor that is not in quorum, bail out of this retry
				logger.Warningf("monitor %s is not in quorum list", name)
				allInQuorum = false
				break
			}
		}

		if allInQuorum {
			logQuorumMembers(monStatusResp)
			break
		}
	}

	return nil
}

func logQuorumMembers(monStatusResp client.MonStatusResponse) {
	var monsInQuorum []string
	for _, m := range monStatusResp.MonMap.Mons {
		if monFoundInQuorum(m.Name, monStatusResp) {
			monsInQuorum = append(monsInQuorum, m.Name)
		}
	}
	logger.Infof("Monitors in quorum: %v", monsInQuorum)
}

func monFoundInQuorum(name string, monStatusResp client.MonStatusResponse) bool {
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
		return false
	}

	// using the current initial monitor's mon map entry, check to see if it's in the quorum list
	// (a list of monitor rank values)
	for _, q := range monStatusResp.Quorum {
		if monMapEntry.Rank == q {
			return true
		}
	}

	return false
}

func (c *Cluster) acquireOrchestrationLock() {
	logger.Debugf("Acquiring lock for mon orchestration")
	c.orchestrationMutex.Lock()
	logger.Debugf("Acquired lock for mon orchestration")
}

func (c *Cluster) releaseOrchestrationLock() {
	c.orchestrationMutex.Unlock()
	logger.Debugf("Released lock for mon orchestration")
}
