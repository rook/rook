/*
Copyright 2018 The Rook Authors. All rights reserved.

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
	"fmt"
	"time"

	"github.com/coreos/pkg/capnslog"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mon")

const (
	// EndpointConfigMapName is the name of the configmap with mon endpoints
	EndpointConfigMapName = "rook-ceph-mon-endpoints"
	// MonEndpointKey is the name of the key inside the mon configmap to get the endpoints
	MonEndpointKey = "mon-endpoints"
	// EndpointKey is the name of the key inside the mon configmap to get the mon dns endpoint
	// it is used for != mon ceph components
	EndpointKey = "endpoints"

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
	Port                int32
	clusterInfo         *mon.ClusterInfo
	placement           rookalpha.Placement
	maxMonID            int
	waitForStart        bool
	dataDirHostPath     string
	monPodRetryInterval time.Duration
	monPodTimeout       time.Duration
	monTimeoutList      map[string]time.Time
	HostNetwork         bool
	resources           v1.ResourceRequirements
	ownerRef            metav1.OwnerReference
}

// monConfig for a single monitor
type monConfig struct {
	Name    string
	Address string
	Port    int32
}

// New creates an instance of a mon cluster
func New(context *clusterd.Context, namespace, dataDirHostPath, version string, size int, placement rookalpha.Placement, hostNetwork bool,
	resources v1.ResourceRequirements, ownerRef metav1.OwnerReference) *Cluster {
	return &Cluster{
		context:             context,
		placement:           placement,
		dataDirHostPath:     dataDirHostPath,
		Namespace:           namespace,
		Version:             version,
		Size:                size,
		waitForStart:        true,
		monPodRetryInterval: 6 * time.Second,
		HostNetwork:         hostNetwork,
		resources:           resources,
		ownerRef:            ownerRef,
	}
}

// Start the mon cluster
func (c *Cluster) Start() error {
	logger.Infof("start running mons")

	if err := c.initClusterInfo(); err != nil {
		return fmt.Errorf("failed to initialize ceph cluster info. %+v", err)
	}

	return c.startMons()
}

func (c *Cluster) startMons() error {
	// create statefulset with replicas 0, the loop below does the scale up to the desired c.Size
	c.startStatefulSet(0)

	mons := []*monConfig{}

	// Init the mons config
	for i := len(c.clusterInfo.Monitors); i < c.Size; i++ {
		logger.Debugf("mon endpoints used are: %s", c.clusterInfo.MonEndpoints())
		// this scales up the mons to the desired c.Size one by one through the loop
		if err := c.updateStatefulSet(int32(i + 1)); err != nil {
			return fmt.Errorf("failed to scale up replicas for mon statefulset to %d. %+v", i+1, err)
		}

		monName := getMonNameForID(i)

		// wait until the mon Pod is ready before we get the IP
		// TODO Maybe change this to only check for Pod is in `Running` state
		if err := c.waitForPodReady(monName); err != nil {
			return fmt.Errorf("failed waiting for mon pod %s being ready. %+v", monName, err)
		}

		// get mon Pod IP and add it to endpoints
		// this is needed as the Mon Pod endpoint watcher is only active after
		// the first full Mon start run
		podIP, err := c.getMonIP(monName)
		if err != nil {
			return fmt.Errorf("failed getting ip from mon pod %s. %+v", monName, err)
		}
		c.clusterInfo.MonMutex.Lock()
		c.clusterInfo.Monitors[monName] = mon.ToCephMon(monName, podIP, mon.DefaultPort)
		c.clusterInfo.MonMutex.Unlock()

		// before we wait for the mon, save the mon IP to the config
		if err := c.saveMonConfig(); err != nil {
			return fmt.Errorf("failed to save mon config. %+v", err)
		}

		logger.Debugf("wait for mons to join (currently at nnumber %d)", i)
		mons = append(mons, &monConfig{Name: monName})
		if err := c.waitForMonsToJoin(mons); err != nil {
			return fmt.Errorf("failed to wait for current mon %s to join. %+v", monName, err)
		}
	}

	if err := c.saveConfigChanges(); err != nil {
		return fmt.Errorf("failed to save mons. %+v", err)
	}

	return nil
}

// Retrieve the ceph cluster info if it already exists.
// If a new cluster create new keys.
func (c *Cluster) initClusterInfo() error {
	var err error
	// get the cluster info from secret
	c.clusterInfo, err = CreateOrLoadClusterInfo(c.context, c.Namespace, &c.ownerRef)
	if err != nil {
		return fmt.Errorf("failed to get cluster info. %+v", err)
	}

	// save cluster monitor config
	if err = c.saveMonConfig(); err != nil {
		return fmt.Errorf("failed to save mons. %+v", err)
	}

	return nil
}

func (c *Cluster) startStatefulSet(replicas int32) error {
	logger.Debug("starting statefulset headless service for mons")
	_, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(c.makeService())
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create mon service. %+v", err)
	}

	logger.Debug("starting statefulset for mons")
	if _, err := c.context.Clientset.AppsV1beta1().StatefulSets(c.Namespace).Create(c.makeStatefulSet(replicas)); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create mons. %+v", err)
		}
		logger.Info("statefulset for mons already exists")
	}
	return nil
}

func (c *Cluster) updateStatefulSet(replicas int32) error {
	logger.Debug("starting statefulset for mons replicas")
	if _, err := c.context.Clientset.AppsV1beta1().StatefulSets(c.Namespace).Update(c.makeStatefulSet(replicas)); err != nil {
		return fmt.Errorf("failed to scale statefulset for mons. %+v", err)
	}
	return nil
}

func (c *Cluster) saveMonConfig() error {
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            EndpointConfigMapName,
			Namespace:       c.Namespace,
			Annotations:     map[string]string{},
			OwnerReferences: []metav1.OwnerReference{c.ownerRef},
		},
	}

	configMap.Data = map[string]string{
		MonEndpointKey: mon.FlattenMonEndpoints(c.clusterInfo.Monitors),
		EndpointKey: mon.FlattenMonEndpoints(map[string]*mon.CephMonitorConfig{
			appName: mon.ToCephMon(appName, c.getMonDNSEndpoint(), mon.DefaultPort),
		}),
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

func (c *Cluster) saveConfigChanges() error {
	if err := c.saveMonConfig(); err != nil {
		return fmt.Errorf("failed to save mon config. %+v", err)
	}

	// make sure to rewrite the config so NO new connections are made to the removed mon
	if err := WriteConnectionConfig(c.context, c.clusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config. %+v", err)
	}

	return nil
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
