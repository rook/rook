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
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

	// AppName is the name of the secret storing cluster mon.admin key, fsid and name
	AppName           = "rook-ceph-mon"
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

	// DefaultMsgr1Port is the default port Ceph mons use to communicate amongst themselves prior
	// to Ceph Nautilus.
	DefaultMsgr1Port int32 = 6789
	// DefaultMsgr2Port is the listening port of the messenger v2 protocol introduced in Ceph
	// Nautilus. In Nautilus and a few Ceph releases after, Ceph can use both v1 and v2 protocols.
	DefaultMsgr2Port int32 = 3300

	// minimum amount of memory in MB to run the pod
	cephMonPodMinimumMemory uint64 = 1024

	// default storage request size for ceph monitor pvc
	// https://docs.ceph.com/docs/master/start/hardware-recommendations/#monitors-and-managers-ceph-mon-and-ceph-mgr
	cephMonDefaultStorageRequest = "10Gi"

	// canary pod scheduling uses retry loops when cleaning up previous canary
	// pods and waiting for kubernetes scheduling to complete.
	canaryRetries           = 180
	canaryRetryDelaySeconds = 5
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mon")

	// hook for tests to override
	scheduleMonitor = realScheduleMonitor
)

// Cluster represents the Rook and environment configuration settings needed to set up Ceph mons.
type Cluster struct {
	ClusterInfo         *cephconfig.ClusterInfo
	context             *clusterd.Context
	spec                cephv1.ClusterSpec
	Namespace           string
	Keyring             string
	rookVersion         string
	orchestrationMutex  sync.Mutex
	Port                int32
	Network             cephv1.NetworkSpec
	maxMonID            int
	waitForStart        bool
	dataDirHostPath     string
	monPodRetryInterval time.Duration
	monPodTimeout       time.Duration
	monTimeoutList      map[string]time.Time
	mapping             *Mapping
	ownerRef            metav1.OwnerReference
	csiConfigMutex      *sync.Mutex
	isUpgrade           bool
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
}

// NodeInfo contains name and address of a node
type NodeInfo struct {
	Name     string
	Hostname string
	Address  string
}

type SchedulingResult struct {
	Node             *v1.Node
	CanaryDeployment string
	CanaryPVC        string
}

// New creates an instance of a mon cluster
func New(context *clusterd.Context, namespace, dataDirHostPath string, network cephv1.NetworkSpec, ownerRef metav1.OwnerReference, csiConfigMutex *sync.Mutex, isUpgrade bool) *Cluster {
	return &Cluster{
		context:             context,
		dataDirHostPath:     dataDirHostPath,
		Namespace:           namespace,
		maxMonID:            -1,
		waitForStart:        true,
		monPodRetryInterval: 6 * time.Second,
		monPodTimeout:       5 * time.Minute,
		monTimeoutList:      map[string]time.Time{},
		Network:             network,
		mapping: &Mapping{
			Node: map[string]*NodeInfo{},
		},
		ownerRef:       ownerRef,
		csiConfigMutex: csiConfigMutex,
		isUpgrade:      isUpgrade,
	}
}

// Start begins the process of running a cluster of Ceph mons.
func (c *Cluster) Start(clusterInfo *cephconfig.ClusterInfo, rookVersion string, cephVersion cephver.CephVersion, spec cephv1.ClusterSpec, isUpgrade bool) (*cephconfig.ClusterInfo, error) {

	// Only one goroutine can orchestrate the mons at a time
	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.ClusterInfo = clusterInfo
	c.rookVersion = rookVersion
	c.spec = spec
	c.isUpgrade = isUpgrade

	// fail if we were instructed to deploy more than one mon on the same machine with host networking
	if c.Network.IsHost() && c.spec.Mon.AllowMultiplePerNode && c.spec.Mon.Count > 1 {
		return nil, errors.Errorf("refusing to deploy %d monitors on the same host since hostNetwork is %+v and allowMultiplePerNode is %t. only one monitor per node is allowed", c.spec.Mon.Count, c.Network, c.spec.Mon.AllowMultiplePerNode)
	}

	// Validate pod's memory if specified
	err := opspec.CheckPodMemory(cephv1.GetMonResources(c.spec.Resources), cephMonPodMinimumMemory)
	if err != nil {
		return nil, errors.Wrap(err, "error checking pod memory")
	}

	logger.Infof("start running mons")

	logger.Debugf("establishing ceph cluster info")
	if err := c.initClusterInfo(cephVersion); err != nil {
		return nil, errors.Wrapf(err, "failed to initialize ceph cluster info")
	}

	logger.Infof("targeting the mon count %d", c.spec.Mon.Count)

	// create the mons for a new cluster or ensure mons are running in an existing cluster
	return c.ClusterInfo, c.startMons(c.spec.Mon.Count)
}

func (c *Cluster) startMons(targetCount int) error {
	// init the mon config
	existingCount, mons := c.initMonConfig(targetCount)

	// Assign the mons to nodes
	if err := c.assignMons(mons); err != nil {
		return errors.Wrapf(err, "failed to assign pods to mons")
	}

	// The centralized mon config database can only be used if there is at least one mon
	// operational. If we are starting mons, and one is already up, then there is a cluster already
	// created, and we can immediately set values in the config database. The goal is to set configs
	// only once and do it as early as possible in the mon orchestration.
	setConfigsNeedsRetry := false
	if existingCount > 0 {
		err := config.SetDefaultConfigs(c.context, c.Namespace, c.ClusterInfo)
		if err != nil {
			// If we fail here, it could be because the mons are not healthy, and this might be
			// fixed by updating the mon deployments. Instead of returning error here, log a
			// warning, and retry setting this later.
			setConfigsNeedsRetry = true
			logger.Warningf("failed to set Rook and/or user-defined Ceph config options before starting mons; will retry after starting mons. %v", err)
		}
	}

	if existingCount < len(mons) {
		// Start the new mons one at a time
		for i := existingCount; i < targetCount; i++ {
			if err := c.ensureMonsRunning(mons, i, targetCount, true); err != nil {
				return err
			}

			// If this is the first mon being created, we have to wait until it is created to set
			// values in the config database. Do this only when the existing count is zero so that
			// this is only done once when the cluster is created.
			if existingCount == 0 {
				err := config.SetDefaultConfigs(c.context, c.Namespace, c.ClusterInfo)
				if err != nil {
					return errors.Wrapf(err, "failed to set Rook and/or user-defined Ceph config options after creating the first mon")
				}
			} else if setConfigsNeedsRetry && i == existingCount {
				// Or if we need to retry, only do this when we are on the first iteration of the
				// loop. This could be in the same if statement as above, but separate it to get a
				// different error message.
				err := config.SetDefaultConfigs(c.context, c.Namespace, c.ClusterInfo)
				if err != nil {
					return errors.Wrapf(err, "failed to set Rook and/or user-defined Ceph config options after updating the existing mons")
				}
			}
		}
	} else {
		// Ensure all the expected mon deployments exist, but don't require full quorum to continue
		lastMonIndex := len(mons) - 1
		if err := c.ensureMonsRunning(mons, lastMonIndex, targetCount, false); err != nil {
			return err
		}

		if setConfigsNeedsRetry {
			err := config.SetDefaultConfigs(c.context, c.Namespace, c.ClusterInfo)
			if err != nil {
				return errors.Wrapf(err, "failed to set Rook and/or user-defined Ceph config options after forcefully updating the existing mons")
			}
		}
	}

	logger.Debugf("mon endpoints used are: %s", FlattenMonEndpoints(c.ClusterInfo.Monitors))
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
	expectedMonCount := len(c.ClusterInfo.Monitors)
	if expectedMonCount < targetCount {
		expectedMonCount++
	}

	// Init the mon IPs
	if err := c.initMonIPs(mons[0:expectedMonCount]); err != nil {
		return errors.Wrapf(err, "failed to init mon services")
	}

	// save the mon config after we have "initiated the IPs"
	if err := c.saveMonConfig(); err != nil {
		return errors.Wrapf(err, "failed to save mons")
	}

	// make sure we have the connection info generated so connections can happen
	if err := WriteConnectionConfig(c.context, c.ClusterInfo); err != nil {
		return err
	}

	// Start the deployment
	if err := c.startDeployments(mons[0:expectedMonCount], requireAllInQuorum); err != nil {
		return errors.Wrapf(err, "failed to start mon pods")
	}

	return nil
}

// initClusterInfo retrieves the ceph cluster info if it already exists.
// If a new cluster, create new keys.
func (c *Cluster) initClusterInfo(cephVersion cephver.CephVersion) error {
	var err error

	// get the cluster info from secret
	c.ClusterInfo, c.maxMonID, c.mapping, err = CreateOrLoadClusterInfo(c.context, c.Namespace, &c.ownerRef)
	if err != nil {
		return errors.Wrapf(err, "failed to get cluster info")
	}

	c.ClusterInfo.CephVersion = cephVersion

	// save cluster monitor config
	if err = c.saveMonConfig(); err != nil {
		return errors.Wrapf(err, "failed to save mons")
	}

	k := keyring.GetSecretStore(c.context, c.Namespace, &c.ownerRef)
	// store the keyring which all mons share
	if err := k.CreateOrUpdate(keyringStoreName, c.genMonSharedKeyring()); err != nil {
		return errors.Wrapf(err, "failed to save mon keyring secret")
	}
	// also store the admin keyring for other daemons that might need it during init
	if err := k.Admin().CreateOrUpdate(c.ClusterInfo); err != nil {
		return errors.Wrapf(err, "failed to save admin keyring secret")
	}

	return nil
}

func (c *Cluster) initMonConfig(size int) (int, []*monConfig) {
	mons := []*monConfig{}

	// initialize the mon pod info for mons that have been previously created
	for _, monitor := range c.ClusterInfo.Monitors {
		mons = append(mons, &monConfig{
			ResourceName: resourceName(monitor.Name),
			DaemonName:   monitor.Name,
			Port:         cephutil.GetPortFromEndpoint(monitor.Endpoint),
			DataPathMap: config.NewStatefulDaemonDataPathMap(
				c.dataDirHostPath, dataDirRelativeHostPath(monitor.Name), config.MonType, monitor.Name, c.Namespace),
		})
	}

	// initialize mon info if we don't have enough mons (at first startup)
	existingCount := len(c.ClusterInfo.Monitors)
	for i := len(c.ClusterInfo.Monitors); i < size; i++ {
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
	if strings.HasPrefix(name, AppName) {
		return name
	}
	return fmt.Sprintf("%s-%s", AppName, name)
}

// scheduleMonitor selects a node for a monitor deployment.
// see startMon() and design/ceph/ceph-mon-pv.md for additional details.
func realScheduleMonitor(c *Cluster, mon *monConfig) (SchedulingResult, error) {
	// target node decision, and deployment/pvc to cleanup
	result := SchedulingResult{
		Node:             nil,
		CanaryDeployment: "",
		CanaryPVC:        "",
	}

	// build the canary deployment.
	d := c.makeDeployment(mon)
	d.Name += "-canary"
	d.Spec.Template.ObjectMeta.Name += "-canary"

	// the canary and real monitor deployments will mount the same storage. to
	// avoid issues with the real deployment, the canary should be careful not
	// to modify the storage by instead running an innocuous command.
	d.Spec.Template.Spec.InitContainers = []v1.Container{}
	d.Spec.Template.Spec.Containers[0].Image = c.rookVersion
	d.Spec.Template.Spec.Containers[0].Command = []string{"/tini"}
	d.Spec.Template.Spec.Containers[0].Args = []string{"--", "sleep", "3600"}

	// setup affinity settings for pod scheduling
	p := cephv1.GetMonPlacement(c.spec.Placement)
	c.setPodPlacement(&d.Spec.Template.Spec, p, nil)

	// setup storage on the canary since scheduling will be affected when
	// monitors are configured to use persistent volumes. the pvcName is set to
	// the non-empty name of the PVC only when the PVC is created as a result of
	// this call to the scheduler.
	var pvcName string
	if c.spec.Mon.VolumeClaimTemplate == nil {
		d.Spec.Template.Spec.Volumes = append(d.Spec.Template.Spec.Volumes,
			opspec.DaemonVolumesDataHostPath(mon.DataPathMap)...)
	} else {
		// the pvc that is created here won't be deleted: it will be reattached
		// to the real monitor deployment.
		pvc, err := c.makeDeploymentPVC(mon)
		if err != nil {
			return result, errors.Wrapf(err, "sched-mon: failed to make monitor %s pvc", d.Name)
		}

		_, err = c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).Create(pvc)
		if err == nil {
			pvcName = pvc.Name
			logger.Infof("sched-mon: created canary monitor %s pvc %s", d.Name, pvc.Name)
		} else {
			if kerrors.IsAlreadyExists(err) {
				logger.Debugf("sched-mon: creating mon %s pvc %s: already exists.", d.Name, pvc.Name)
			} else {
				return result, errors.Wrapf(err, "sched-mon: error creating mon %s pvc %s", d.Name, pvc.Name)
			}
		}

		d.Spec.Template.Spec.Volumes = append(d.Spec.Template.Spec.Volumes,
			opspec.DaemonVolumesDataPVC(mon.ResourceName))
		opspec.AddVolumeMountSubPath(&d.Spec.Template.Spec, "ceph-daemon-data")
	}

	// caller should arrange for clean-up of the PVC only if it was created
	// during this scheduling instance and we encounter an irrecoverable error.
	result.CanaryPVC = pvcName

	// spin up the canary deployment. if it exists, delete it first, since if it
	// already exists it may have been scheduled with a different crd config.
	createdDeployment := false
	for i := 0; i < canaryRetries; i++ {
		_, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Create(d)
		if err == nil {
			createdDeployment = true
			logger.Infof("sched-mon: created canary deployment %s", d.Name)
			break
		} else if kerrors.IsAlreadyExists(err) {
			if err := k8sutil.DeleteDeployment(c.context.Clientset, c.Namespace, d.Name); err != nil {
				return result, errors.Wrapf(err, "sched-mon: error deleting canary deployment %s", d.Name)
			}
			logger.Infof("sched-mon: deleted existing canary deployment %s", d.Name)
			time.Sleep(time.Second * canaryRetryDelaySeconds)
		} else {
			return result, errors.Wrapf(err, "sched-mon: error creating canary monitor deployment %s", d.Name)
		}
	}

	// failed after retrying
	if !createdDeployment {
		return result, errors.Errorf("sched-mon: failed to create canary deployment %s", d.Name)
	}

	// caller should arrange for the deployment to be removed
	result.CanaryDeployment = d.Name

	// wait for the scheduler to make a placement decision
	for i := 0; i < canaryRetries; i++ {
		listOptions := metav1.ListOptions{
			LabelSelector: labels.Set(d.Spec.Selector.MatchLabels).String(),
		}

		pods, err := c.context.Clientset.CoreV1().Pods(c.Namespace).List(listOptions)
		if err != nil {
			return result, errors.Wrapf(err, "sched-mon: error listing canary pods %s", d.Name)
		}

		if len(pods.Items) == 0 {
			logger.Infof("sched-mon: waiting for canary pod creation %s", d.Name)
			time.Sleep(time.Second * canaryRetryDelaySeconds)
			continue
		}

		pod := pods.Items[0]
		if pod.Spec.NodeName == "" {
			logger.Debugf("sched-mon: monitor %s canary pod %s not yet scheduled", d.Name, pod.Name)
			time.Sleep(time.Second * canaryRetryDelaySeconds)
			continue
		}

		node, err := c.context.Clientset.CoreV1().Nodes().Get(pod.Spec.NodeName, metav1.GetOptions{})
		if err != nil {
			return result, errors.Wrapf(err, "sched-mon: error getting node %s", pod.Spec.NodeName)
		}

		result.Node = node
		result.CanaryPVC = ""
		logger.Infof("sched-mon: canary monitor deployment %s scheduled to %s", d.Name, node.Name)
		return result, nil
	}

	return result, errors.New("sched-mon: canary pod scheduling failed retries")
}

func (c *Cluster) initMonIPs(mons []*monConfig) error {
	for _, m := range mons {
		if c.Network.IsHost() {
			logger.Infof("setting mon endpoints for hostnetwork mode")
			node, ok := c.mapping.Node[m.DaemonName]
			if !ok {
				return errors.New("mon doesn't exist in assignment map")
			}
			m.PublicIP = node.Address
		} else {
			serviceIP, err := c.createService(m)
			if err != nil {
				return errors.Wrapf(err, "failed to create mon service")
			}
			m.PublicIP = serviceIP
		}
		c.ClusterInfo.Monitors[m.DaemonName] = cephconfig.NewMonInfo(m.DaemonName, m.PublicIP, m.Port)
	}

	return nil
}

func (c *Cluster) assignMons(mons []*monConfig) error {
	// when monitors are scheduling below by invoking scheduleMonitor() a canary
	// deployment and optional canary PVC are created. in order for the
	// anti-affinity rules to be effective, we leave the canary pods in place
	// until all of the canaries have been scheduled. only after the
	// monitor/node assignment process is complete are the canary deployments
	// and pvcs removed here.
	canaryCleanup := []SchedulingResult{}
	defer func() {
		for _, result := range canaryCleanup {
			logger.Infof("assignmon: cleaning up canary deployment %s and canary pvc %s", result.CanaryDeployment, result.CanaryPVC)
			if result.CanaryDeployment != "" {
				if err := k8sutil.DeleteDeployment(c.context.Clientset, c.Namespace, result.CanaryDeployment); err != nil {
					logger.Infof("assignmon: error deleting canary deployment %s. %v", result.CanaryDeployment, err)
				}
			}
			if result.CanaryPVC != "" {
				var gracePeriod int64 // delete immediately
				propagation := metav1.DeletePropagationForeground
				options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
				err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).Delete(result.CanaryPVC, options)
				if err != nil {
					logger.Infof("assignmon: error removing canary monitor %s pvc %s. %v", result.CanaryDeployment, result.CanaryPVC, err)
				}
			}
		}
	}()

	// ensure that all monitors have either (1) a node assignment that will be
	// enforced using a node selector, or (2) configuration permits k8s to handle
	// scheduling for the monitor.
	for _, mon := range mons {

		// scheduling for this monitor has already been completed
		if _, ok := c.mapping.Node[mon.DaemonName]; ok {
			logger.Debugf("assignmon: mon %s already scheduled", mon.DaemonName)
			continue
		}

		// determine a placement for the monitor. note that this scheduling is
		// performed even when a node selector is not required. this may be
		// non-optimal, but it is convenient to catch some failures early,
		// before a decision is stored in the node mapping.
		result, err := scheduleMonitor(c, mon)

		// even if an error occurs cleanup may be necessary
		canaryCleanup = append(canaryCleanup, result)

		if err != nil {
			return errors.Wrapf(err, "assignmon: error scheduling monitor")
		}

		nodeChoice := result.Node
		if nodeChoice == nil {
			return errors.Errorf("assignmon: could not schedule monitor %s", mon.DaemonName)
		}

		// store nil in the node mapping to indicate that an explicit node
		// placement is not being made. otherwise, the node choice will map
		// directly to a node selector on the monitor pod.
		var nodeInfo *NodeInfo = nil
		if c.Network.IsHost() || c.spec.Mon.VolumeClaimTemplate == nil {
			logger.Infof("assignmon: mon %s assigned to node %s", mon.DaemonName, nodeChoice.Name)
			nodeInfo, err = getNodeInfoFromNode(*nodeChoice)
			if err != nil {
				return errors.Wrapf(err, "assignmon: couldn't get node info for node %s", nodeChoice.Name)
			}
		} else {
			logger.Infof("assignmon: mon %s placement using native scheduler", mon.DaemonName)
		}

		c.mapping.Node[mon.DaemonName] = nodeInfo
	}

	logger.Debug("assignmons: mons have been scheduled")
	return nil
}

func (c *Cluster) startDeployments(mons []*monConfig, requireAllInQuorum bool) error {
	if len(mons) == 0 {
		return errors.New("cannot start 0 mons")
	}

	// If all the mon deployments don't exist, allow the mon deployments to all be started without checking for quorum.
	// This will be the case where:
	// 1) New clusters where we are starting one deployment at a time. We only need to check for quorum once when we add a new mon.
	// 2) Clusters being restored where no mon deployments are running. We need to start all the deployments before checking quorum.
	onlyCheckQuorumOnce := false
	deployments, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).List(metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", AppName)})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Infof("0 of %d expected mon deployments exist. creating new deployment(s).", len(mons))
			onlyCheckQuorumOnce = true
		} else {
			logger.Warningf("failed to list mon deployments. attempting to continue. %v", err)
		}
	}

	readyReplicas := 0
	// Ensuring the mon deployments should be ready
	for _, deploy := range deployments.Items {
		if deploy.Status.AvailableReplicas > 0 {
			readyReplicas++
		}
	}
	if len(deployments.Items) < len(mons) {
		logger.Infof("%d of %d expected mon deployments exist. creating new deployment(s).", len(deployments.Items), len(mons))
		onlyCheckQuorumOnce = true
	} else if readyReplicas == 0 {
		logger.Infof("%d of %d expected mons are ready. creating or updating deployments without checking quorum in attempt to achieve a healthy mon cluster", readyReplicas, len(mons))
		onlyCheckQuorumOnce = true
	}

	// Ensure each of the mons have been created. If already created, it will be a no-op.
	for i := 0; i < len(mons); i++ {
		node, _ := c.mapping.Node[mons[i].DaemonName]
		err := c.startMon(mons[i], node)
		if err != nil {
			if c.isUpgrade {
				// if we're upgrading, we don't want to risk the health of the cluster by continuing to upgrade
				// and potentially cause more mons to fail. Therefore, we abort if the mon failed to start after upgrade.
				return errors.Wrapf(err, "failed to upgrade mon %q.", mons[i].DaemonName)
			}
			// We will attempt to start all mons, then check for quorum as needed after this. During an operator restart
			// we need to do everything possible to verify the basic health of a cluster, complete the first orchestration,
			// and start watching for all the CRs. If mons still have quorum we can continue with the orchestration even
			// if they aren't all up.
			logger.Errorf("attempting to continue after failing to start mon %q. %v", mons[i].DaemonName, err)
		}

		// For the initial deployment (first creation) it's expected to not have all the monitors in quorum
		// However, in an event of an update, it's crucial to proceed monitors by monitors
		// At the end of the method we perform one last check where all the monitors must be in quorum
		if !onlyCheckQuorumOnce || (onlyCheckQuorumOnce && i == len(mons)-1) {
			requireAllInQuorum := false
			err = c.waitForMonsToJoin(mons, requireAllInQuorum)
			if err != nil {
				return errors.Wrapf(err, "failed to check mon quorum %s", mons[i].DaemonName)
			}
		}
	}

	logger.Infof("mons created: %d", len(mons))
	// Final verification that **all** mons are in quorum
	// Do not proceed if one monitor is still syncing
	// Only do this when monitors versions are different so we don't block the orchestration if a mon is down.
	versions, err := client.GetAllCephDaemonVersions(c.context, c.ClusterInfo.Name)
	if err != nil {
		logger.Warningf("failed to get ceph daemons versions; this likely means there is no cluster yet. %v", err)
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
	err := waitForQuorumWithMons(c.context, c.ClusterInfo.Name, starting, sleepTime, requireAllInQuorum)
	if err != nil {
		return errors.Wrapf(err, "failed to wait for mon quorum")
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
		return errors.Wrapf(err, "failed to marshal mon mapping")
	}

	csiConfigValue, err := csi.FormatCsiClusterConfig(
		c.Namespace, c.ClusterInfo.Monitors)
	if err != nil {
		return errors.Wrapf(err, "failed to format csi config")
	}

	configMap.Data = map[string]string{
		EndpointDataKey: FlattenMonEndpoints(c.ClusterInfo.Monitors),
		MaxMonIDKey:     strconv.Itoa(c.maxMonID),
		MappingKey:      string(monMapping),
		csi.ConfigKey:   csiConfigValue,
	}

	if _, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(configMap); err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create mon endpoint config map")
		}

		logger.Debugf("updating config map %s that already exists", configMap.Name)
		if _, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Update(configMap); err != nil {
			return errors.Wrapf(err, "failed to update mon endpoint config map")
		}
	}

	logger.Infof("saved mon endpoints to config map %+v", configMap.Data)

	// Every time the mon config is updated, must also update the global config so that all daemons
	// have the most updated version if they restart.
	config.GetStore(c.context, c.Namespace, &c.ownerRef).CreateOrUpdate(c.ClusterInfo)

	// write the latest config to the config dir
	if err := WriteConnectionConfig(c.context, c.ClusterInfo); err != nil {
		return errors.Wrapf(err, "failed to write connection config for new mons")
	}

	if err := csi.SaveClusterConfig(c.context.Clientset, c.Namespace, c.ClusterInfo, c.csiConfigMutex); err != nil {
		return errors.Wrapf(err, "failed to update csi cluster config")
	}

	return nil
}

var updateDeploymentAndWait = UpdateCephDeploymentAndWait

func (c *Cluster) updateMon(m *monConfig, d *apps.Deployment) error {
	logger.Infof("deployment for mon %s already exists. updating if needed",
		d.Name)

	daemonType := string(config.MonType)
	var cephVersionToUse cephver.CephVersion

	// If this is not a Ceph upgrade there is no need to check the ceph version
	if c.isUpgrade {
		currentCephVersion, err := client.LeastUptodateDaemonVersion(c.context, c.ClusterInfo.Name, daemonType)
		if err != nil {
			logger.Warningf("failed to retrieve current ceph %q version. %v", daemonType, err)
			logger.Debug("could not detect ceph version during update, this is likely an initial bootstrap, proceeding with %+v", c.ClusterInfo.CephVersion)
			cephVersionToUse = c.ClusterInfo.CephVersion
		} else {
			logger.Debugf("current cluster version for monitors before upgrading is: %+v", currentCephVersion)
			cephVersionToUse = currentCephVersion
		}
	}

	err := updateDeploymentAndWait(c.context, d, c.Namespace, daemonType, m.DaemonName, cephVersionToUse, c.isUpgrade, c.spec.SkipUpgradeChecks, false)
	if err != nil {
		return errors.Wrapf(err, "failed to update mon deployment %s", m.ResourceName)
	}

	return nil
}

// startMon creates or updates a monitor deployment.
//
// The node parameter specifies the node to be used as a node selector on the
// monitor pod. It is the result of scheduling a canary pod: see
// scheduleMonitor() for more details on scheduling.
//
// The node parameter is optional. When the parameter is nil it indicates that
// the pod should not use a node selector, and should instead rely on k8s to
// perform scheduling.
//
// The following outlines the different scenarios that exist and how deployments
// should be configured w.r.t. scheduling and the use of a node selector.
//
// 1) if HostNetworking -> always use node selector. we do not want to change
//    the IP address of a monitor as it is wrapped up in the monitor's identity.
//    with host networking we use node selector to ensure a stable IP for each
//    monitor. see scheduleMonitor() comment for more details.
//
// Note: an important assumption is that HostNetworking setting does not
// change once a cluster is created.
//
// 2) if *not* HostNetworking -> stable IP from service; may avoid node selector
//      a) when creating a new deployment
//           - if HostPath -> use node selector for storage/node affinity
//           - if PVC      -> node selector is not required
//
//      b) when updating a deployment
//           - if HostPath -> leave node selector as is
//           - if PVC      -> remove node selector, if present
//
func (c *Cluster) startMon(m *monConfig, node *NodeInfo) error {
	// check if the monitor deployment already exists. if the deployment does
	// exist, also determine if it using pvc storage.
	pvcExists := false
	deploymentExists := false
	d := c.makeDeployment(m)
	existingDeployment, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(d.Name, metav1.GetOptions{})
	if err == nil {
		deploymentExists = true
		pvcExists = opspec.DaemonVolumesContainsPVC(existingDeployment.Spec.Template.Spec.Volumes)
	} else if !kerrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to get mon deployment %s", d.Name)
	}

	// persistent storage is not altered after the deployment is created. this
	// means we need to be careful when updating the deployment to avoid new
	// changes to the crd to change an existing pod's persistent storage. the
	// deployment spec created above does not specify persistent storage. here
	// we add in PVC or HostPath storage based on an existing deployment OR on
	// the current state of the CRD.
	if pvcExists || (!deploymentExists && c.spec.Mon.VolumeClaimTemplate != nil) {
		pvcName := m.ResourceName
		d.Spec.Template.Spec.Volumes = append(d.Spec.Template.Spec.Volumes, opspec.DaemonVolumesDataPVC(pvcName))
		opspec.AddVolumeMountSubPath(&d.Spec.Template.Spec, "ceph-daemon-data")
		logger.Debugf("adding pvc volume source %s to mon deployment %s", pvcName, d.Name)
	} else {
		d.Spec.Template.Spec.Volumes = append(d.Spec.Template.Spec.Volumes, opspec.DaemonVolumesDataHostPath(m.DataPathMap)...)
		logger.Debugf("adding host path volume source to mon deployment %s", d.Name)
	}

	// placement settings from the CRD
	p := cephv1.GetMonPlacement(c.spec.Placement)

	if deploymentExists {
		// the existing deployment may have a node selector. if the cluster
		// isn't using host networking and the deployment is using pvc storage,
		// then the node selector can be removed. this may happen after
		// upgrading the cluster with the k8s scheduling support for monitors.
		if c.Network.IsHost() || !pvcExists {
			p.PodAffinity = nil
			p.PodAntiAffinity = nil
			c.setPodPlacement(&d.Spec.Template.Spec, p,
				existingDeployment.Spec.Template.Spec.NodeSelector)
		} else {
			c.setPodPlacement(&d.Spec.Template.Spec, p, nil)
		}
		return c.updateMon(m, d)
	}

	if c.spec.Mon.VolumeClaimTemplate != nil {
		pvc, err := c.makeDeploymentPVC(m)
		if err != nil {
			return errors.Wrapf(err, "failed to make mon %s pvc", d.Name)
		}
		_, err = c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).Create(pvc)
		if err != nil {
			if kerrors.IsAlreadyExists(err) {
				logger.Debugf("cannot create mon %s pvc %s: already exists.", d.Name, pvc.Name)
			} else {
				return errors.Wrapf(err, "failed to create mon %s pvc %s", d.Name, pvc.Name)
			}
		}
	}

	if node == nil {
		c.setPodPlacement(&d.Spec.Template.Spec, p, nil)
	} else {
		p.PodAffinity = nil
		p.PodAntiAffinity = nil
		c.setPodPlacement(&d.Spec.Template.Spec, p,
			map[string]string{v1.LabelHostname: node.Hostname})
	}

	logger.Debugf("Starting mon: %+v", d.Name)
	_, err = c.context.Clientset.AppsV1().Deployments(c.Namespace).Create(d)
	if err != nil {
		return errors.Wrapf(err, "failed to create mon deployment %s", d.Name)
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
			return errors.New("exceeded max retry count waiting for monitors to reach quorum")
		}

		if retryCount > 1 {
			// only sleep after the first time
			<-time.After(time.Duration(sleepTime) * time.Second)
		}

		// wait for the mon pods to be running
		allPodsRunning := true
		var runningMonNames []string
		for _, m := range mons {
			running, err := k8sutil.PodsRunningWithLabel(context.Clientset, clusterName, fmt.Sprintf("app=%s,mon=%s", AppName, m))
			if err != nil {
				logger.Infof("failed to query mon pod status, trying again. %v", err)
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

		// get the quorum_status response that contains info about all monitors in the mon map and
		// their quorum status
		monQuorumStatusResp, err := client.GetMonQuorumStatus(context, clusterName, false)
		if err != nil {
			logger.Debugf("failed to get quorum_status. %v", err)
			continue
		}

		if !requireAllInQuorum {
			logQuorumMembers(monQuorumStatusResp)
			break
		}

		// check if each of the initial monitors is in quorum
		allInQuorum := true
		for _, name := range mons {
			if !monFoundInQuorum(name, monQuorumStatusResp) {
				// found an initial monitor that is not in quorum, bail out of this retry
				logger.Warningf("monitor %s is not in quorum list", name)
				allInQuorum = false
				break
			}
		}

		if allInQuorum {
			logQuorumMembers(monQuorumStatusResp)
			break
		}
	}

	return nil
}

func logQuorumMembers(monQuorumStatusResp client.MonStatusResponse) {
	var monsInQuorum []string
	for _, m := range monQuorumStatusResp.MonMap.Mons {
		if monFoundInQuorum(m.Name, monQuorumStatusResp) {
			monsInQuorum = append(monsInQuorum, m.Name)
		}
	}
	logger.Infof("Monitors in quorum: %v", monsInQuorum)
}

func monFoundInQuorum(name string, monQuorumStatusResp client.MonStatusResponse) bool {
	// first get the initial monitors corresponding mon map entry
	var monMapEntry *client.MonMapEntry
	for i := range monQuorumStatusResp.MonMap.Mons {
		if name == monQuorumStatusResp.MonMap.Mons[i].Name {
			monMapEntry = &monQuorumStatusResp.MonMap.Mons[i]
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
	for _, q := range monQuorumStatusResp.Quorum {
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
