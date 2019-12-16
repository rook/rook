/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package target

import (
	"fmt"
	"os"
	"time"

	"github.com/coreos/pkg/capnslog"
	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-target")

const (
	appName                   = "rook-edgefs-target"
	targetAppNameFmt          = "rook-edgefs-target-id-%d"
	appNameFmt                = "rook-edgefs-target-%s"
	targetLabelKey            = "edgefs-target-id"
	defaultServiceAccountName = "rook-edgefs-cluster"
	labelingRetries           = 5
	nodeTypeLabelFmt          = "%s-nodetype"
	sleepTime                 = 5 // time beetween statefulset update check
)

// Cluster keeps track of the Targets
type Cluster struct {
	context          *clusterd.Context
	Namespace        string
	annotations      rookalpha.Annotations
	placement        rookalpha.Placement
	Version          string
	Storage          rookalpha.StorageScopeSpec
	dataDirHostPath  string
	dataVolumeSize   resource.Quantity
	NetworkSpec      rookalpha.NetworkSpec
	Privileged       bool
	resources        v1.ResourceRequirements
	resourceProfile  string
	chunkCacheSize   resource.Quantity
	ownerRef         metav1.OwnerReference
	serviceAccount   string
	deploymentConfig edgefsv1.ClusterDeploymentConfig
	useHostLocalTime bool
}

// New creates an instance of the Target manager
func New(
	context *clusterd.Context,
	namespace,
	version,
	serviceAccount string,
	storageSpec rookalpha.StorageScopeSpec,
	dataDirHostPath string,
	dataVolumeSize resource.Quantity,
	annotations rookalpha.Annotations,
	placement rookalpha.Placement,
	NetworkSpec rookalpha.NetworkSpec,
	resources v1.ResourceRequirements,
	resourceProfile string,
	chunkCacheSize resource.Quantity,
	ownerRef metav1.OwnerReference,
	deploymentConfig edgefsv1.ClusterDeploymentConfig,
	useHostLocalTime bool,
) *Cluster {

	if serviceAccount == "" {
		// if the service account was not set, make a best effort with the example service account name since the default is unlikely to be sufficient.
		serviceAccount = defaultServiceAccountName
		logger.Infof("setting the target pods to use the service account name: %s", serviceAccount)
	}
	return &Cluster{
		context:          context,
		Namespace:        namespace,
		serviceAccount:   serviceAccount,
		annotations:      annotations,
		placement:        placement,
		Version:          version,
		Storage:          storageSpec,
		dataDirHostPath:  dataDirHostPath,
		dataVolumeSize:   dataVolumeSize,
		NetworkSpec:      NetworkSpec,
		Privileged:       (NetworkSpec.IsHost() || os.Getenv("ROOK_HOSTPATH_REQUIRES_PRIVILEGED") == "true"),
		resources:        resources,
		resourceProfile:  resourceProfile,
		chunkCacheSize:   chunkCacheSize,
		ownerRef:         ownerRef,
		deploymentConfig: deploymentConfig,
		useHostLocalTime: useHostLocalTime,
	}
}

// Start the target management
func (c *Cluster) Start(rookImage string, nodes []rookalpha.Node, dro edgefsv1.DevicesResurrectOptions) (err error) {
	logger.Infof("start running targets in namespace %s", c.Namespace)

	logger.Infof("Target Image is %s", rookImage)

	headlessService, _ := c.makeHeadlessService()
	if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(headlessService); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("headless service %s already exists in namespace %s", headlessService.Name, headlessService.Namespace)
	} else {
		logger.Infof("headless service %s started in namespace %s", headlessService.Name, headlessService.Namespace)
	}

	statefulSet, _ := c.makeStatefulSet(int32(len(nodes)), rookImage, dro)
	if _, err := c.context.Clientset.AppsV1().StatefulSets(c.Namespace).Create(statefulSet); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("stateful set %s already exists in namespace %s", statefulSet.Name, statefulSet.Namespace)
		logger.Infof("Trying to update statefulset %s", statefulSet.Name)

		if _, err := UpdateStatefulsetAndWait(c.context, statefulSet, c.Namespace); err != nil {
			logger.Errorf("failed to update statefulset %s. %+v", statefulSet.Name, err)
			return err
		}
	} else {
		logger.Infof("stateful set %s created in namespace %s", statefulSet.Name, statefulSet.Namespace)
	}
	return nil
}

// UpdateStatefulsetAndWait updates a statefulset and waits until it is running to return. It will
// error if the statefulset does not exist to be updated or if it takes too long.
func UpdateStatefulsetAndWait(context *clusterd.Context, sts *appsv1.StatefulSet, namespace string) (*appsv1.StatefulSet, error) {
	original, err := context.Clientset.AppsV1().StatefulSets(namespace).Get(sts.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get statefulset %s. %+v", sts.Name, err)
	}

	// set updateTime annotation to force rolling update of Statefulset
	sts.Spec.Template.Annotations["UpdateTime"] = time.Now().Format(time.RFC850)

	_, err = context.Clientset.AppsV1().StatefulSets(namespace).Update(sts)
	if err != nil {
		return nil, fmt.Errorf("failed to update statefulset %s. %+v", sts.Name, err)
	}
	// wait for the statefulset to be restarted
	sleepTime := 5
	attempts := 24 * int(original.Status.Replicas) // 2 minutes per replica
	for i := 0; i < attempts; i++ {
		// check for the status of the statefulset
		statefulset, err := context.Clientset.AppsV1().StatefulSets(namespace).Get(sts.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get statefulset %s. %+v", statefulset.Name, err)
		}

		logger.Infof("Statefulset %s update in progress... status=%+v", statefulset.Name, statefulset.Status)
		statefulsetReplicas := *statefulset.Spec.Replicas
		if statefulset.Status.ObservedGeneration != original.Status.ObservedGeneration &&
			statefulsetReplicas == statefulset.Status.ReadyReplicas &&
			statefulsetReplicas == statefulset.Status.CurrentReplicas &&
			statefulsetReplicas == statefulset.Status.UpdatedReplicas {
			logger.Infof("Statefulset '%s' update is done", statefulset.Name)
			return statefulset, nil
		}
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}

	return nil, fmt.Errorf("gave up waiting for statefulset %s to update", sts.Name)
}
