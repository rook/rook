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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/
package operator

import (
	"fmt"
	"time"

	"github.com/kubernetes-incubator/external-storage/lib/controller"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/cluster"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/kit"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	initRetryDelay = 10 * time.Second
)

// volume provisioner constant
const (
	resyncPeriod              = 15 * time.Second
	provisionerName           = "rook.io/block"
	exponentialBackOffOnError = false
	failedRetryThreshold      = 5
	leasePeriod               = controller.DefaultLeaseDuration
	retryPeriod               = controller.DefaultRetryPeriod
	renewDeadline             = controller.DefaultRenewDeadline
	termLimit                 = controller.DefaultTermLimit
)

type Operator struct {
	context   *clusterd.Context
	resources []kit.CustomResource
	// The custom resource that is global to the kubernetes cluster.
	// The cluster is global because you create multiple clusers in k8s
	clusterMgr        *clusterManager
	volumeProvisioner controller.Provisioner
}

type inclusterInitiator interface {
	Create(clusterMgr *clusterManager, namespace string) (resourceManager, error)
	Resource() kit.CustomResource
}

type resourceManager interface {
	Load() (string, error)
	Manage()
}

func New(context *clusterd.Context) *Operator {

	poolInitiator := newPoolInitiator(context)
	clusterMgr := newClusterManager(context, []inclusterInitiator{poolInitiator})
	volumeProvisioner := newRookVolumeProvisioner(clusterMgr)

	schemes := []kit.CustomResource{cluster.ClusterResource, cluster.PoolResource}
	return &Operator{
		context:           context,
		clusterMgr:        clusterMgr,
		resources:         schemes,
		volumeProvisioner: volumeProvisioner,
	}
}

func (o *Operator) Run() error {

	for {
		err := o.initResources()
		if err == nil {
			break
		}
		logger.Errorf("failed to init resources. %+v. retrying...", err)
		<-time.After(initRetryDelay)
	}

	// Run volume provisioner
	// The controller needs to know what the server version is because out-of-tree
	// provisioners aren't officially supported until 1.5
	serverVersion, err := o.context.Clientset.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("Error getting server version: %v", err)
	}
	pc := controller.NewProvisionController(
		o.context.Clientset,
		resyncPeriod,
		provisionerName,
		o.volumeProvisioner,
		serverVersion.GitVersion,
		exponentialBackOffOnError,
		failedRetryThreshold,
		leasePeriod,
		renewDeadline,
		retryPeriod,
		termLimit)
	go pc.Run(wait.NeverStop)

	// watch for changes to the rook clusters
	o.clusterMgr.Manage()
	return nil
}

func (o *Operator) initResources() error {
	httpCli, err := kit.NewHTTPClient(k8sutil.CustomResourceGroup)
	if err != nil {
		return fmt.Errorf("failed to get tpr client. %+v", err)
	}
	o.context.KubeHTTPCli = httpCli.Client

	err = kit.CreateCustomResources(o.context.KubeContext, o.resources)
	if err != nil {
		return fmt.Errorf("failed to create TPR. %+v", err)
	}

	return nil
}
