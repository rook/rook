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

// Package mgr for the Ceph manager.
package mgr

import (
	"fmt"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opspec "github.com/rook/rook/pkg/operator/ceph/spec"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-mgr")

const (
	appName              = "rook-ceph-mgr"
	serviceAccountName   = "rook-ceph-mgr"
	prometheusModuleName = "prometheus"
	metricsPort          = 9283
	// minimum amount of memory in MB to run the pod
	cephMgrPodMinimumMemory uint64 = 512
)

// Cluster represents the Rook and environment configuration settings needed to set up Ceph mgrs.
type Cluster struct {
	clusterInfo     *cephconfig.ClusterInfo
	Namespace       string
	Replicas        int
	placement       rookalpha.Placement
	annotations     rookalpha.Annotations
	context         *clusterd.Context
	dataDir         string
	HostNetwork     bool
	resources       v1.ResourceRequirements
	ownerRef        metav1.OwnerReference
	dashboard       cephv1.DashboardSpec
	cephVersion     cephv1.CephVersionSpec
	rookVersion     string
	exitCode        func(err error) (int, bool)
	dataDirHostPath string
}

// New creates an instance of the mgr
func New(
	clusterInfo *cephconfig.ClusterInfo,
	context *clusterd.Context,
	namespace, rookVersion string,
	cephVersion cephv1.CephVersionSpec,
	placement rookalpha.Placement,
	annotations rookalpha.Annotations,
	hostNetwork bool,
	dashboard cephv1.DashboardSpec,
	resources v1.ResourceRequirements,
	ownerRef metav1.OwnerReference,
	dataDirHostPath string,
) *Cluster {
	return &Cluster{
		clusterInfo:     clusterInfo,
		context:         context,
		Namespace:       namespace,
		placement:       placement,
		rookVersion:     rookVersion,
		cephVersion:     cephVersion,
		Replicas:        1,
		dataDir:         k8sutil.DataDir,
		dashboard:       dashboard,
		HostNetwork:     hostNetwork,
		resources:       resources,
		ownerRef:        ownerRef,
		exitCode:        getExitCode,
		dataDirHostPath: dataDirHostPath,
	}
}

var updateDeploymentAndWait = k8sutil.UpdateDeploymentAndWait

// Start begins the process of running a cluster of Ceph mgrs.
func (c *Cluster) Start() error {
	// Validate pod's memory if specified
	err := opspec.CheckPodMemory(c.resources, cephMgrPodMinimumMemory)
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	logger.Infof("start running mgr")

	for i := 0; i < c.Replicas; i++ {
		if i >= 2 {
			logger.Errorf("cannot have more than 2 mgrs")
			break
		}

		daemonID := k8sutil.IndexToName(i)
		resourceName := fmt.Sprintf("%s-%s", appName, daemonID)
		mgrConfig := &mgrConfig{
			DaemonID:      daemonID,
			ResourceName:  resourceName,
			DashboardPort: c.dashboardPort(),
			DataPathMap:   config.NewStatelessDaemonDataPathMap(config.MgrType, daemonID, c.Namespace, c.dataDirHostPath),
		}

		// generate keyring specific to this mgr daemon saved to k8s secret
		if err := c.generateKeyring(mgrConfig); err != nil {
			return fmt.Errorf("failed to generate keyring for %s. %+v", resourceName, err)
		}

		if !c.needHttpBindFix() {
			c.clearHttpBindFix(mgrConfig)
		}

		// start the deployment
		d := c.makeDeployment(mgrConfig)
		logger.Debugf("starting mgr deployment: %+v", d)
		_, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Create(d)
		if err != nil {
			if !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create mgr deployment %s. %+v", resourceName, err)
			}
			logger.Infof("deployment for mgr %s already exists. updating if needed", resourceName)
			if _, err := updateDeploymentAndWait(c.context, d, c.Namespace); err != nil {
				return fmt.Errorf("failed to update mgr deployment %s. %+v", resourceName, err)
			}
		}

		if err := c.configureOrchestratorModules(); err != nil {
			logger.Errorf("failed to enable orchestrator modules. %+v", err)
		}

		if err := c.enablePrometheusModule(c.Namespace); err != nil {
			logger.Errorf("failed to enable mgr prometheus module. %+v", err)
		}

		if err := c.configureDashboard(mgrConfig); err != nil {
			logger.Errorf("failed to enable mgr dashboard. %+v", err)
		}

	}

	// create the metrics service
	service := c.makeMetricsService(appName)
	if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(service); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create mgr service. %+v", err)
		}
		logger.Infof("mgr metrics service already exists")
	} else {
		logger.Infof("mgr metrics service started")
	}

	return nil
}

// Ceph docs about the prometheus module: http://docs.ceph.com/docs/master/mgr/prometheus/
func (c *Cluster) enablePrometheusModule(clusterName string) error {
	if err := client.MgrEnableModule(c.context, clusterName, prometheusModuleName, true); err != nil {
		return fmt.Errorf("failed to enable mgr prometheus module. %+v", err)
	}
	return nil
}
