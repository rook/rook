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
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
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
	AppName = "rook-ceph-mon"
	// OperatorCreds is the name of the secret
	OperatorCreds     = "rook-ceph-operator-creds"
	monClusterAttr    = "mon_cluster"
	fsidSecretNameKey = "fsid"
	monSecretNameKey  = "mon-secret"
	// AdminSecretName is the name of the admin secret
	adminSecretNameKey = "admin-secret"
	cephUsernameKey    = "ceph-username"
	cephUserSecretKey  = "ceph-secret"

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
	canaryRetries           = 30
	canaryRetryDelaySeconds = 5
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mon")

	// hook for tests to override
	waitForMonitorScheduling = realWaitForMonitorScheduling
)

// Cluster represents the Rook and environment configuration settings needed to set up Ceph mons.
type Cluster struct {
	ClusterInfo         *cephclient.ClusterInfo
	context             *clusterd.Context
	spec                cephv1.ClusterSpec
	Namespace           string
	Keyring             string
	rookVersion         string
	orchestrationMutex  sync.Mutex
	Port                int32
	maxMonID            int
	waitForStart        bool
	monPodRetryInterval time.Duration
	monPodTimeout       time.Duration
	monTimeoutList      map[string]time.Time
	mapping             *Mapping
	ownerInfo           *cephclient.OwnerInfo
	csiConfigMutex      *sync.Mutex
	isUpgrade           bool
	arbiterMon          string
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
	// The zone used for a stretch cluster
	Zone string
	// DataPathMap is the mapping relationship between mon data stored on the host and mon data
	// stored in containers.
	DataPathMap *config.DataPathMap
}

// Mapping is mon node and port mapping
type Mapping struct {
	// This isn't really node info since it could also be for zones, but we leave it as "node" for backward compatibility.
	Schedule map[string]*MonScheduleInfo `json:"node"`
}

// MonScheduleInfo contains name and address of a node.
type MonScheduleInfo struct {
	// Name of the node. **json names are capitalized for backwards compat**
	Name     string `json:"Name,omitempty"`
	Hostname string `json:"Hostname,omitempty"`
	Address  string `json:"Address,omitempty"`
	Zone     string `json:"zone,omitempty"`
}

type SchedulingResult struct {
	Node             *v1.Node
	CanaryDeployment *apps.Deployment
	CanaryPVC        string
}

// New creates an instance of a mon cluster
func New(context *clusterd.Context, namespace string, spec cephv1.ClusterSpec, ownerInfo *cephclient.OwnerInfo, csiConfigMutex *sync.Mutex) *Cluster {
	return &Cluster{
		context:             context,
		spec:                spec,
		Namespace:           namespace,
		maxMonID:            -1,
		waitForStart:        true,
		monPodRetryInterval: 6 * time.Second,
		monPodTimeout:       5 * time.Minute,
		monTimeoutList:      map[string]time.Time{},
		mapping: &Mapping{
			Schedule: map[string]*MonScheduleInfo{},
		},
		ownerInfo:      ownerInfo,
		csiConfigMutex: csiConfigMutex,
	}
}

// Start begins the process of running a cluster of Ceph mons.
func (c *Cluster) Start(clusterInfo *cephclient.ClusterInfo, rookVersion string, cephVersion cephver.CephVersion, spec cephv1.ClusterSpec) (*cephclient.ClusterInfo, error) {

	// Only one goroutine can orchestrate the mons at a time
	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	clusterInfo.OwnerInfo = *c.ownerInfo
	c.ClusterInfo = clusterInfo
	c.rookVersion = rookVersion
	c.spec = spec

	// fail if we were instructed to deploy more than one mon on the same machine with host networking
	if c.spec.Network.IsHost() && c.spec.Mon.AllowMultiplePerNode && c.spec.Mon.Count > 1 {
		return nil, errors.Errorf("refusing to deploy %d monitors on the same host with host networking and allowMultiplePerNode is %t. only one monitor per node is allowed", c.spec.Mon.Count, c.spec.Mon.AllowMultiplePerNode)
	}

	// Validate pod's memory if specified
	err := controller.CheckPodMemory(cephv1.ResourcesKeyMon, cephv1.GetMonResources(c.spec.Resources), cephMonPodMinimumMemory)
	if err != nil {
		return nil, errors.Wrap(err, "error checking pod memory")
	}

	logger.Infof("start running mons")

	logger.Debugf("establishing ceph cluster info")
	if err := c.initClusterInfo(cephVersion); err != nil {
		return nil, errors.Wrap(err, "failed to initialize ceph cluster info")
	}

	logger.Infof("targeting the mon count %d", c.spec.Mon.Count)

	// create the mons for a new cluster or ensure mons are running in an existing cluster
	return c.ClusterInfo, c.startMons(c.spec.Mon.Count)
}

func (c *Cluster) startMons(targetCount int) error {
	// init the mon config
	existingCount, mons, err := c.initMonConfig(targetCount)
	if err != nil {
		return errors.Wrap(err, "failed to init mon config")
	}

	// Assign the mons to nodes
	if err := c.assignMons(mons); err != nil {
		return errors.Wrap(err, "failed to assign pods to mons")
	}

	// The centralized mon config database can only be used if there is at least one mon
	// operational. If we are starting mons, and one is already up, then there is a cluster already
	// created, and we can immediately set values in the config database. The goal is to set configs
	// only once and do it as early as possible in the mon orchestration.
	setConfigsNeedsRetry := false
	if existingCount > 0 {
		err := config.SetDefaultConfigs(c.context, c.ClusterInfo, c.spec.Network)
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
			// Check whether we need to cancel the orchestration
			if err := controller.CheckForCancelledOrchestration(c.context); err != nil {
				return err
			}

			if err := c.ensureMonsRunning(mons, i, targetCount, true); err != nil {
				return err
			}

			// If this is the first mon being created, we have to wait until it is created to set
			// values in the config database. Do this only when the existing count is zero so that
			// this is only done once when the cluster is created.
			if existingCount == 0 {
				err := config.SetDefaultConfigs(c.context, c.ClusterInfo, c.spec.Network)
				if err != nil {
					return errors.Wrap(err, "failed to set Rook and/or user-defined Ceph config options after creating the first mon")
				}
			} else if setConfigsNeedsRetry && i == existingCount {
				// Or if we need to retry, only do this when we are on the first iteration of the
				// loop. This could be in the same if statement as above, but separate it to get a
				// different error message.
				err := config.SetDefaultConfigs(c.context, c.ClusterInfo, c.spec.Network)
				if err != nil {
					return errors.Wrap(err, "failed to set Rook and/or user-defined Ceph config options after updating the existing mons")
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
			err := config.SetDefaultConfigs(c.context, c.ClusterInfo, c.spec.Network)
			if err != nil {
				return errors.Wrap(err, "failed to set Rook and/or user-defined Ceph config options after forcefully updating the existing mons")
			}
		}
	}

	if c.spec.IsStretchCluster() {
		if err := c.configureStretchCluster(mons); err != nil {
			return errors.Wrap(err, "failed to configure stretch mons")
		}
	}

	logger.Debugf("mon endpoints used are: %s", FlattenMonEndpoints(c.ClusterInfo.Monitors))

	// Check if there are orphaned mon resources that should be cleaned up at the end of a reconcile.
	// There may be orphaned resources if a mon failover was aborted.
	c.removeOrphanMonResources()

	return nil
}

func (c *Cluster) configureStretchCluster(mons []*monConfig) error {
	if err := c.assignStretchMonsToZones(mons); err != nil {
		return errors.Wrap(err, "failed to assign mons to zones")
	}

	// Enable the mon connectivity strategy
	if err := client.EnableStretchElectionStrategy(c.context, c.ClusterInfo); err != nil {
		return errors.Wrap(err, "failed to enable stretch cluster")
	}

	// Create the default crush rule for stretch clusters, that by default will also apply to all pools
	if err := client.CreateDefaultStretchCrushRule(c.context, c.ClusterInfo, &c.spec, c.stretchFailureDomainName()); err != nil {
		return errors.Wrap(err, "failed to create default stretch rule")
	}

	return nil
}

func (c *Cluster) assignStretchMonsToZones(mons []*monConfig) error {
	var arbiterZone string
	for _, zone := range c.spec.Mon.StretchCluster.Zones {
		if zone.Arbiter {
			arbiterZone = zone.Name
			break
		}
	}

	// Set the location for each mon
	domainName := c.stretchFailureDomainName()
	for _, mon := range mons {
		if mon.Zone == arbiterZone {
			// remember the arbiter mon to be set later in the reconcile after the OSDs are configured
			c.arbiterMon = mon.DaemonName
		}
		logger.Infof("setting mon %q to stretch %s=%s", mon.DaemonName, domainName, mon.Zone)
		if err := client.SetMonStretchZone(c.context, c.ClusterInfo, mon.DaemonName, domainName, mon.Zone); err != nil {
			return errors.Wrapf(err, "failed to set mon %q zone", mon.DaemonName)
		}
	}
	return nil
}

func (c *Cluster) ConfigureArbiter() error {
	if c.arbiterMon == "" {
		return errors.New("arbiter not specified for the stretch cluster")
	}

	// Set the mon tiebreaker
	if err := client.SetMonStretchTiebreaker(c.context, c.ClusterInfo, c.arbiterMon, c.stretchFailureDomainName()); err != nil {
		return errors.Wrap(err, "failed to set mon tiebreaker")
	}

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
		return errors.Wrap(err, "failed to init mon services")
	}

	// save the mon config after we have "initiated the IPs"
	if err := c.saveMonConfig(); err != nil {
		return errors.Wrap(err, "failed to save mons")
	}

	// make sure we have the connection info generated so connections can happen
	if err := WriteConnectionConfig(c.context, c.ClusterInfo); err != nil {
		return err
	}

	// Start the deployment
	if err := c.startDeployments(mons[0:expectedMonCount], requireAllInQuorum); err != nil {
		return errors.Wrap(err, "failed to start mon pods")
	}

	return nil
}

// initClusterInfo retrieves the ceph cluster info if it already exists.
// If a new cluster, create new keys.
func (c *Cluster) initClusterInfo(cephVersion cephver.CephVersion) error {
	var err error

	// get the cluster info from secret
	c.ClusterInfo, c.maxMonID, c.mapping, err = CreateOrLoadClusterInfo(c.context, c.Namespace, c.ownerInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get cluster info")
	}

	c.ClusterInfo.CephVersion = cephVersion
	c.ClusterInfo.OwnerInfo = *c.ownerInfo

	// save cluster monitor config
	if err = c.saveMonConfig(); err != nil {
		return errors.Wrap(err, "failed to save mons")
	}

	k := keyring.GetSecretStore(c.context, c.ClusterInfo, &c.ClusterInfo.OwnerInfo)
	// store the keyring which all mons share
	if err := k.CreateOrUpdate(keyringStoreName, c.genMonSharedKeyring()); err != nil {
		return errors.Wrap(err, "failed to save mon keyring secret")
	}
	// also store the admin keyring for other daemons that might need it during init
	if err := k.Admin().CreateOrUpdate(c.ClusterInfo); err != nil {
		return errors.Wrap(err, "failed to save admin keyring secret")
	}

	return nil
}

func (c *Cluster) initMonConfig(size int) (int, []*monConfig, error) {

	// initialize the mon pod info for mons that have been previously created
	mons := c.clusterInfoToMonConfig("")

	// initialize mon info if we don't have enough mons (at first startup)
	existingCount := len(c.ClusterInfo.Monitors)
	for i := len(c.ClusterInfo.Monitors); i < size; i++ {
		c.maxMonID++
		zone, err := c.findAvailableZoneIfStretched(mons)
		if err != nil {
			return existingCount, mons, errors.Wrap(err, "stretch zone not available")
		}
		mons = append(mons, c.newMonConfig(c.maxMonID, zone))
	}

	return existingCount, mons, nil
}

func (c *Cluster) clusterInfoToMonConfig(excludedMon string) []*monConfig {
	mons := []*monConfig{}
	for _, monitor := range c.ClusterInfo.Monitors {
		if monitor.Name == excludedMon {
			// Skip a mon if it is being failed over
			continue
		}
		var zone string
		schedule := c.mapping.Schedule[monitor.Name]
		if schedule != nil {
			zone = schedule.Zone
		}
		mons = append(mons, &monConfig{
			ResourceName: resourceName(monitor.Name),
			DaemonName:   monitor.Name,
			Port:         cephutil.GetPortFromEndpoint(monitor.Endpoint),
			PublicIP:     cephutil.GetIPFromEndpoint(monitor.Endpoint),
			Zone:         zone,
			DataPathMap: config.NewStatefulDaemonDataPathMap(
				c.spec.DataDirHostPath, dataDirRelativeHostPath(monitor.Name), config.MonType, monitor.Name, c.Namespace),
		})
	}
	return mons
}

func (c *Cluster) newMonConfig(monID int, zone string) *monConfig {
	daemonName := k8sutil.IndexToName(monID)

	return &monConfig{
		ResourceName: resourceName(daemonName),
		DaemonName:   daemonName,
		Port:         DefaultMsgr1Port,
		Zone:         zone,
		DataPathMap: config.NewStatefulDaemonDataPathMap(
			c.spec.DataDirHostPath, dataDirRelativeHostPath(daemonName), config.MonType, daemonName, c.Namespace),
	}
}

func (c *Cluster) findAvailableZoneIfStretched(mons []*monConfig) (string, error) {
	if !c.spec.IsStretchCluster() {
		return "", nil
	}

	// Build the count of current mons per zone
	zoneCount := map[string]int{}
	for _, m := range mons {
		if m.Zone == "" {
			return "", errors.Errorf("zone not found on mon %q", m.DaemonName)
		}
		zoneCount[m.Zone]++
	}

	// Find a zone in the stretch cluster that still needs an assignment
	for _, zone := range c.spec.Mon.StretchCluster.Zones {
		count, ok := zoneCount[zone.Name]
		if !ok {
			// The zone isn't currently assigned to any mon, so return it
			return zone.Name, nil
		}
		if c.spec.Mon.Count == 5 && count == 1 && !zone.Arbiter {
			// The zone only has 1 mon assigned, but needs 2 mons since it is not the arbiter
			return zone.Name, nil
		}
	}
	return "", errors.New("A zone is not available to assign a new mon")
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
func scheduleMonitor(c *Cluster, mon *monConfig) (*apps.Deployment, error) {
	ctx := context.TODO()
	// build the canary deployment.
	d, err := c.makeDeployment(mon, true)
	if err != nil {
		return nil, err
	}
	d.Name += "-canary"
	d.Spec.Template.ObjectMeta.Name += "-canary"

	// the canary and real monitor deployments will mount the same storage. to
	// avoid issues with the real deployment, the canary should be careful not
	// to modify the storage by instead running an innocuous command.
	d.Spec.Template.Spec.InitContainers = []v1.Container{}
	d.Spec.Template.Spec.Containers[0].Image = c.rookVersion
	d.Spec.Template.Spec.Containers[0].Command = []string{"/tini"}
	d.Spec.Template.Spec.Containers[0].Args = []string{"--", "sleep", "3600"}
	// remove the liveness probe on the canary pod
	d.Spec.Template.Spec.Containers[0].LivenessProbe = nil

	// setup affinity settings for pod scheduling
	p := cephv1.GetMonPlacement(c.spec.Placement)
	k8sutil.SetNodeAntiAffinityForPod(&d.Spec.Template.Spec, p, requiredDuringScheduling(&c.spec),
		map[string]string{k8sutil.AppAttr: AppName}, nil)

	// setup storage on the canary since scheduling will be affected when
	// monitors are configured to use persistent volumes. the pvcName is set to
	// the non-empty name of the PVC only when the PVC is created as a result of
	// this call to the scheduler.
	if c.monVolumeClaimTemplate(mon) == nil {
		d.Spec.Template.Spec.Volumes = append(d.Spec.Template.Spec.Volumes,
			controller.DaemonVolumesDataHostPath(mon.DataPathMap)...)
	} else {
		// the pvc that is created here won't be deleted: it will be reattached
		// to the real monitor deployment.
		pvc, err := c.makeDeploymentPVC(mon, true)
		if err != nil {
			return nil, errors.Wrapf(err, "sched-mon: failed to make monitor %s pvc", d.Name)
		}

		_, err = c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
		if err == nil {
			logger.Infof("sched-mon: created canary monitor %s pvc %s", d.Name, pvc.Name)
		} else {
			if kerrors.IsAlreadyExists(err) {
				logger.Debugf("sched-mon: creating mon %s pvc %s: already exists.", d.Name, pvc.Name)
			} else {
				return nil, errors.Wrapf(err, "sched-mon: error creating mon %s pvc %s", d.Name, pvc.Name)
			}
		}

		d.Spec.Template.Spec.Volumes = append(d.Spec.Template.Spec.Volumes,
			controller.DaemonVolumesDataPVC(mon.ResourceName))
		controller.AddVolumeMountSubPath(&d.Spec.Template.Spec, "ceph-daemon-data")
	}

	// spin up the canary deployment. if it exists, delete it first, since if it
	// already exists it may have been scheduled with a different crd config.
	createdDeployment := false
	for i := 0; i < canaryRetries; i++ {
		_, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Create(ctx, d, metav1.CreateOptions{})
		if err == nil {
			createdDeployment = true
			logger.Infof("sched-mon: created canary deployment %s", d.Name)
			break
		} else if kerrors.IsAlreadyExists(err) {
			if err := k8sutil.DeleteDeployment(c.context.Clientset, c.Namespace, d.Name); err != nil {
				return nil, errors.Wrapf(err, "sched-mon: error deleting canary deployment %s", d.Name)
			}
			logger.Infof("sched-mon: deleted existing canary deployment %s", d.Name)
			time.Sleep(time.Second * canaryRetryDelaySeconds)
		} else {
			return nil, errors.Wrapf(err, "sched-mon: error creating canary monitor deployment %s", d.Name)
		}
	}

	// failed after retrying
	if !createdDeployment {
		return nil, errors.Errorf("sched-mon: failed to create canary deployment %s", d.Name)
	}

	// caller should arrange for the deployment to be removed
	return d, nil
}

func realWaitForMonitorScheduling(c *Cluster, d *apps.Deployment) (SchedulingResult, error) {
	ctx := context.TODO()
	// target node decision, and deployment/pvc to cleanup
	result := SchedulingResult{}

	// wait for the scheduler to make a placement decision
	for i := 0; i < canaryRetries; i++ {
		if i > 0 {
			time.Sleep(time.Second * canaryRetryDelaySeconds)
		}

		listOptions := metav1.ListOptions{
			LabelSelector: labels.Set(d.Spec.Selector.MatchLabels).String(),
		}

		pods, err := c.context.Clientset.CoreV1().Pods(c.Namespace).List(ctx, listOptions)
		if err != nil {
			return result, errors.Wrapf(err, "sched-mon: error listing canary pods %s", d.Name)
		}

		if len(pods.Items) == 0 {
			logger.Infof("sched-mon: waiting for canary pod creation %s", d.Name)
			continue
		}

		pod := pods.Items[0]
		if pod.Spec.NodeName == "" {
			logger.Debugf("sched-mon: monitor %s canary pod %s not yet scheduled", d.Name, pod.Name)
			continue
		}

		node, err := c.context.Clientset.CoreV1().Nodes().Get(ctx, pod.Spec.NodeName, metav1.GetOptions{})
		if err != nil {
			return result, errors.Wrapf(err, "sched-mon: error getting node %s", pod.Spec.NodeName)
		}

		result.Node = node
		logger.Infof("sched-mon: canary monitor deployment %s scheduled to %s", d.Name, node.Name)
		return result, nil
	}

	return result, errors.New("sched-mon: canary pod scheduling failed retries")
}

func (c *Cluster) initMonIPs(mons []*monConfig) error {
	for _, m := range mons {
		if c.spec.Network.IsHost() {
			logger.Infof("setting mon endpoints for hostnetwork mode")
			node, ok := c.mapping.Schedule[m.DaemonName]
			if !ok {
				return errors.New("mon doesn't exist in assignment map")
			}
			m.PublicIP = node.Address
		} else {
			serviceIP, err := c.createService(m)
			if err != nil {
				return errors.Wrap(err, "failed to create mon service")
			}
			m.PublicIP = serviceIP
		}
		c.ClusterInfo.Monitors[m.DaemonName] = cephclient.NewMonInfo(m.DaemonName, m.PublicIP, m.Port)
	}

	return nil
}

// Delete mon canary deployments (and associated PVCs) using deployment labels
// to select this kind of temporary deployments
func (c *Cluster) removeCanaryDeployments() {
	ctx := context.TODO()
	canaryDeployments, err := k8sutil.GetDeployments(c.context.Clientset, c.Namespace, "app=rook-ceph-mon,mon_canary=true")
	if err != nil {
		logger.Warningf("failed to get the list of monitor canary deployments. %v", err)
		return
	}

	// Delete the canary mons, but don't wait for them to exit
	for _, canary := range canaryDeployments.Items {
		logger.Infof("cleaning up canary monitor deployment %q", canary.Name)
		var gracePeriod int64
		propagation := metav1.DeletePropagationForeground
		options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
		if err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Delete(ctx, canary.Name, *options); err != nil {
			logger.Warningf("failed to delete canary monitor deployment %q. %v", canary.Name, err)
		}
	}
}

func (c *Cluster) assignMons(mons []*monConfig) error {
	// when monitors are scheduling below by invoking scheduleMonitor() a canary
	// deployment and optional canary PVC are created. In order for the
	// anti-affinity rules to be effective, we leave the canary pods in place
	// until all of the canaries have been scheduled. Only after the
	// monitor/node assignment process is complete are the canary deployments
	// and pvcs removed here.
	defer c.removeCanaryDeployments()

	var monSchedulingWait sync.WaitGroup
	var resultLock sync.Mutex
	failedMonSchedule := false

	// ensure that all monitors have either (1) a node assignment that will be
	// enforced using a node selector, or (2) configuration permits k8s to handle
	// scheduling for the monitor.
	for _, mon := range mons {
		// Check whether we need to cancel the orchestration
		if err := controller.CheckForCancelledOrchestration(c.context); err != nil {
			return err
		}

		// scheduling for this monitor has already been completed
		if _, ok := c.mapping.Schedule[mon.DaemonName]; ok {
			logger.Debugf("assignmon: mon %s already scheduled", mon.DaemonName)
			continue
		}

		// determine a placement for the monitor. note that this scheduling is
		// performed even when a node selector is not required. this may be
		// non-optimal, but it is convenient to catch some failures early,
		// before a decision is stored in the node mapping.
		deployment, err := scheduleMonitor(c, mon)
		if err != nil {
			return errors.Wrap(err, "assignmon: error scheduling monitor")
		}

		// start waiting for the deployment
		monSchedulingWait.Add(1)

		go func(deployment *apps.Deployment, mon *monConfig) {
			// signal that the mon is done scheduling
			defer monSchedulingWait.Done()

			result, err := waitForMonitorScheduling(c, deployment)
			if err != nil {
				logger.Errorf("failed to schedule mon %q. %v", mon.DaemonName, err)
				failedMonSchedule = true
				return
			}

			nodeChoice := result.Node
			if nodeChoice == nil {
				logger.Errorf("assignmon: could not schedule monitor %q", mon.DaemonName)
				failedMonSchedule = true
				return
			}

			// store nil in the node mapping to indicate that an explicit node
			// placement is not being made. otherwise, the node choice will map
			// directly to a node selector on the monitor pod.
			var schedule *MonScheduleInfo
			if c.spec.Network.IsHost() || c.monVolumeClaimTemplate(mon) == nil {
				logger.Infof("assignmon: mon %s assigned to node %s", mon.DaemonName, nodeChoice.Name)
				schedule, err = getNodeInfoFromNode(*nodeChoice)
				if err != nil {
					logger.Errorf("assignmon: couldn't get node info for node %q. %v", nodeChoice.Name, err)
					failedMonSchedule = true
					return
				}
			} else {
				logger.Infof("assignmon: mon %q placement using native scheduler", mon.DaemonName)
			}

			if c.spec.IsStretchCluster() {
				if schedule == nil {
					schedule = &MonScheduleInfo{}
				}
				logger.Infof("mon %q is assigned to zone %q", mon.DaemonName, mon.Zone)
				schedule.Zone = mon.Zone
			}

			// protect against multiple goroutines updating the status at the same time
			resultLock.Lock()
			c.mapping.Schedule[mon.DaemonName] = schedule
			resultLock.Unlock()
		}(deployment, mon)
	}

	monSchedulingWait.Wait()
	if failedMonSchedule {
		return errors.New("failed to schedule mons")
	}

	logger.Debug("assignmons: mons have been scheduled")
	return nil
}

func (c *Cluster) monVolumeClaimTemplate(mon *monConfig) *v1.PersistentVolumeClaim {
	if !c.spec.IsStretchCluster() {
		return c.spec.Mon.VolumeClaimTemplate
	}

	// If a stretch cluster, a zone can override the template from the default.
	for _, zone := range c.spec.Mon.StretchCluster.Zones {
		if zone.Name == mon.Zone {
			if zone.VolumeClaimTemplate != nil {
				// Found an override for the volume claim template in the zone
				return zone.VolumeClaimTemplate
			}
			break
		}
	}
	// Return the default template since one wasn't found in the zone
	return c.spec.Mon.VolumeClaimTemplate
}

func (c *Cluster) startDeployments(mons []*monConfig, requireAllInQuorum bool) error {
	ctx := context.TODO()
	if len(mons) == 0 {
		return errors.New("cannot start 0 mons")
	}

	// If all the mon deployments don't exist, allow the mon deployments to all be started without checking for quorum.
	// This will be the case where:
	// 1) New clusters where we are starting one deployment at a time. We only need to check for quorum once when we add a new mon.
	// 2) Clusters being restored where no mon deployments are running. We need to start all the deployments before checking quorum.
	onlyCheckQuorumOnce := false
	deployments, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", AppName)})
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
		schedule := c.mapping.Schedule[mons[i].DaemonName]
		err := c.startMon(mons[i], schedule)
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
	versions, err := client.GetAllCephDaemonVersions(c.context, c.ClusterInfo)
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
	err := waitForQuorumWithMons(c.context, c.ClusterInfo, starting, sleepTime, requireAllInQuorum)
	if err != nil {
		return errors.Wrap(err, "failed to wait for mon quorum")
	}

	return nil
}

func (c *Cluster) saveMonConfig() error {
	ctx := context.TODO()
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      EndpointConfigMapName,
			Namespace: c.Namespace,
		},
	}
	err := c.ownerInfo.SetOwnerReference(configMap)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference of configmap %q", configMap.Name)
	}
	monMapping, err := json.Marshal(c.mapping)
	if err != nil {
		return errors.Wrap(err, "failed to marshal mon mapping")
	}

	csiConfigValue, err := csi.FormatCsiClusterConfig(
		c.Namespace, c.ClusterInfo.Monitors)
	if err != nil {
		return errors.Wrap(err, "failed to format csi config")
	}

	configMap.Data = map[string]string{
		EndpointDataKey: FlattenMonEndpoints(c.ClusterInfo.Monitors),
		MaxMonIDKey:     strconv.Itoa(c.maxMonID),
		MappingKey:      string(monMapping),
		csi.ConfigKey:   csiConfigValue,
	}

	if _, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(ctx, configMap, metav1.CreateOptions{}); err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "failed to create mon endpoint config map")
		}

		logger.Debugf("updating config map %s that already exists", configMap.Name)
		if _, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Update(ctx, configMap, metav1.UpdateOptions{}); err != nil {
			return errors.Wrap(err, "failed to update mon endpoint config map")
		}
	}

	logger.Infof("saved mon endpoints to config map %+v", configMap.Data)

	// Every time the mon config is updated, must also update the global config so that all daemons
	// have the most updated version if they restart.
	if err := config.GetStore(c.context, c.Namespace, c.ownerInfo).CreateOrUpdate(c.ClusterInfo); err != nil {
		return errors.Wrap(err, "failed to update the global config")
	}

	// write the latest config to the config dir
	if err := WriteConnectionConfig(c.context, c.ClusterInfo); err != nil {
		return errors.Wrap(err, "failed to write connection config for new mons")
	}

	if err := csi.SaveClusterConfig(c.context.Clientset, c.Namespace, c.ClusterInfo, c.csiConfigMutex); err != nil {
		return errors.Wrap(err, "failed to update csi cluster config")
	}

	return nil
}

var updateDeploymentAndWait = UpdateCephDeploymentAndWait

func (c *Cluster) updateMon(m *monConfig, d *apps.Deployment) error {
	logger.Infof("deployment for mon %s already exists. updating if needed",
		d.Name)

	// Restart the mon if it is stuck on a failed node
	if err := c.restartMonIfStuckTerminating(m.DaemonName); err != nil {
		logger.Error("failed to restart mon if it is stuck", err)
	}

	err := updateDeploymentAndWait(c.context, c.ClusterInfo, d, config.MonType, m.DaemonName, c.spec.SkipUpgradeChecks, false)
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
func (c *Cluster) startMon(m *monConfig, schedule *MonScheduleInfo) error {
	ctx := context.TODO()
	// check if the monitor deployment already exists. if the deployment does
	// exist, also determine if it using pvc storage.
	pvcExists := false
	deploymentExists := false

	d, err := c.makeDeployment(m, false)
	if err != nil {
		return err
	}

	// Set the deployment hash as an annotation
	err = patch.DefaultAnnotator.SetLastAppliedAnnotation(d)
	if err != nil {
		return errors.Wrapf(err, "failed to set annotation for deployment %q", d.Name)
	}

	existingDeployment, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(ctx, d.Name, metav1.GetOptions{})
	if err == nil {
		deploymentExists = true
		pvcExists = controller.DaemonVolumesContainsPVC(existingDeployment.Spec.Template.Spec.Volumes)
	} else if !kerrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to get mon deployment %s", d.Name)
	}

	// persistent storage is not altered after the deployment is created. this
	// means we need to be careful when updating the deployment to avoid new
	// changes to the crd to change an existing pod's persistent storage. the
	// deployment spec created above does not specify persistent storage. here
	// we add in PVC or HostPath storage based on an existing deployment OR on
	// the current state of the CRD.
	if pvcExists || (!deploymentExists && c.monVolumeClaimTemplate(m) != nil) {
		pvcName := m.ResourceName
		d.Spec.Template.Spec.Volumes = append(d.Spec.Template.Spec.Volumes, controller.DaemonVolumesDataPVC(pvcName))
		controller.AddVolumeMountSubPath(&d.Spec.Template.Spec, "ceph-daemon-data")
		logger.Debugf("adding pvc volume source %s to mon deployment %s", pvcName, d.Name)
	} else {
		d.Spec.Template.Spec.Volumes = append(d.Spec.Template.Spec.Volumes, controller.DaemonVolumesDataHostPath(m.DataPathMap)...)
		logger.Debugf("adding host path volume source to mon deployment %s", d.Name)
	}

	// placement settings from the CRD
	p := cephv1.GetMonPlacement(c.spec.Placement)

	if deploymentExists {
		// the existing deployment may have a node selector. if the cluster
		// isn't using host networking and the deployment is using pvc storage,
		// then the node selector can be removed. this may happen after
		// upgrading the cluster with the k8s scheduling support for monitors.
		if c.spec.Network.IsHost() || !pvcExists {
			p.PodAffinity = nil
			p.PodAntiAffinity = nil
			k8sutil.SetNodeAntiAffinityForPod(&d.Spec.Template.Spec, p, requiredDuringScheduling(&c.spec),
				map[string]string{k8sutil.AppAttr: AppName}, existingDeployment.Spec.Template.Spec.NodeSelector)
		} else {
			k8sutil.SetNodeAntiAffinityForPod(&d.Spec.Template.Spec, p, requiredDuringScheduling(&c.spec),
				map[string]string{k8sutil.AppAttr: AppName}, nil)
		}
		return c.updateMon(m, d)
	}

	if c.monVolumeClaimTemplate(m) != nil {
		pvc, err := c.makeDeploymentPVC(m, false)
		if err != nil {
			return errors.Wrapf(err, "failed to make mon %s pvc", d.Name)
		}
		_, err = c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
		if err != nil {
			if kerrors.IsAlreadyExists(err) {
				logger.Debugf("cannot create mon %s pvc %s: already exists.", d.Name, pvc.Name)
			} else {
				return errors.Wrapf(err, "failed to create mon %s pvc %s", d.Name, pvc.Name)
			}
		}
	}

	if schedule == nil {
		k8sutil.SetNodeAntiAffinityForPod(&d.Spec.Template.Spec, p, requiredDuringScheduling(&c.spec),
			map[string]string{k8sutil.AppAttr: AppName}, nil)
	} else {
		p.PodAffinity = nil
		p.PodAntiAffinity = nil
		k8sutil.SetNodeAntiAffinityForPod(&d.Spec.Template.Spec, p, requiredDuringScheduling(&c.spec),
			map[string]string{k8sutil.AppAttr: AppName}, map[string]string{v1.LabelHostname: schedule.Hostname})
	}

	logger.Debugf("Starting mon: %+v", d.Name)
	_, err = c.context.Clientset.AppsV1().Deployments(c.Namespace).Create(ctx, d, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to create mon deployment %s", d.Name)
	}

	return nil
}

func waitForQuorumWithMons(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, mons []string, sleepTime int, requireAllInQuorum bool) error {
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
			running, err := k8sutil.PodsRunningWithLabel(context.Clientset, clusterInfo.Namespace, fmt.Sprintf("app=%s,mon=%s", AppName, m))
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
		monQuorumStatusResp, err := client.GetMonQuorumStatus(context, clusterInfo)
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

func requiredDuringScheduling(spec *cephv1.ClusterSpec) bool {
	return spec.Network.IsHost() || !spec.Mon.AllowMultiplePerNode
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
