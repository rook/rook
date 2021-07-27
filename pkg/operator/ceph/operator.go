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

// Package operator to manage Kubernetes storage.
package operator

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/operator/ceph/agent"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/ceph/provisioner"
	"github.com/rook/rook/pkg/operator/discover"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/v6/controller"
)

// volume provisioner constant
const (
	provisionerName       = "ceph.rook.io/block"
	provisionerNameLegacy = "rook.io/block"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "operator")

	// The supported configurations for the volume provisioner
	provisionerConfigs = map[string]string{
		provisionerName:       flexvolume.FlexvolumeVendor,
		provisionerNameLegacy: flexvolume.FlexvolumeVendorLegacy,
	}

	// ImmediateRetryResult Return this for a immediate retry of the reconciliation loop with the same request object.
	ImmediateRetryResult = reconcile.Result{Requeue: true}
)

// Operator type for managing storage
type Operator struct {
	context           *clusterd.Context
	resources         []k8sutil.CustomResource
	operatorNamespace string
	rookImage         string
	securityAccount   string
	// The custom resource that is global to the kubernetes cluster.
	// The cluster is global because you create multiple clusters in k8s
	clusterController     *cluster.ClusterController
	delayedDaemonsStarted bool
}

// New creates an operator instance
func New(context *clusterd.Context, volumeAttachmentWrapper attachment.Attachment, rookImage, securityAccount string) *Operator {
	schemes := []k8sutil.CustomResource{opcontroller.ClusterResource, attachment.VolumeResource}

	operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	o := &Operator{
		context:           context,
		resources:         schemes,
		operatorNamespace: operatorNamespace,
		rookImage:         rookImage,
		securityAccount:   securityAccount,
	}
	operatorConfigCallbacks := []func() error{
		o.updateDrivers,
		o.updateOperatorLogLevel,
	}
	addCallbacks := []func() error{
		o.startDrivers,
	}
	o.clusterController = cluster.NewClusterController(context, rookImage, volumeAttachmentWrapper, operatorConfigCallbacks, addCallbacks)
	return o
}

func (o *Operator) cleanup(stopCh chan struct{}) {
	close(stopCh)
	o.clusterController.StopWatch()
}

func (o *Operator) updateOperatorLogLevel() error {
	rookLogLevel, err := k8sutil.GetOperatorSetting(o.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_LOG_LEVEL", "INFO")
	if err != nil {
		logger.Warningf("failed to load ROOK_LOG_LEVEL. Defaulting to INFO. %v", err)
		rookLogLevel = "INFO"
	}

	logLevel, err := capnslog.ParseLevel(strings.ToUpper(rookLogLevel))
	if err != nil {
		return errors.Wrapf(err, "failed to load ROOK_LOG_LEVEL %q.", rookLogLevel)
	}

	capnslog.SetGlobalLogLevel(logLevel)
	return nil
}

// Run the operator instance
func (o *Operator) Run() error {

	if o.operatorNamespace == "" {
		return errors.Errorf("rook operator namespace is not provided. expose it via downward API in the rook operator manifest file using environment variable %q", k8sutil.PodNamespaceEnvVar)
	}

	opcontroller.SetCephCommandsTimeout(o.context)
	// creating a context
	stopContext, stopFunc := context.WithCancel(context.Background())
	defer stopFunc()

	rookDiscover := discover.New(o.context.Clientset)
	if opcontroller.DiscoveryDaemonEnabled(o.context) {
		if err := rookDiscover.Start(o.operatorNamespace, o.rookImage, o.securityAccount, true); err != nil {
			return errors.Wrap(err, "failed to start device discovery daemonset")
		}
	} else {
		if err := rookDiscover.Stop(stopContext, o.operatorNamespace); err != nil {
			return errors.Wrap(err, "failed to stop device discovery daemonset")
		}
	}

	logger.Debug("checking for admission controller secrets")
	err := StartControllerIfSecretPresent(stopContext, o.context, o.rookImage)
	if err != nil {
		return errors.Wrap(err, "failed to start webhook")
	}
	serverVersion, err := o.context.Clientset.Discovery().ServerVersion()
	if err != nil {
		return errors.Wrap(err, "failed to get server version")
	}

	// Initialize signal handler
	signalChan := make(chan os.Signal, 1)
	stopChan := make(chan struct{})
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// For Flex Driver, run volume provisioner for each of the supported configurations
	if opcontroller.FlexDriverEnabled(o.context) {
		for name, vendor := range provisionerConfigs {
			volumeProvisioner := provisioner.New(o.context, vendor)
			pc := controller.NewProvisionController(
				o.context.Clientset,
				name,
				volumeProvisioner,
				serverVersion.GitVersion,
			)
			go pc.Run(stopContext)
			logger.Infof("rook-provisioner %q started using %q flex vendor dir", name, vendor)
		}
	}

	var namespaceToWatch string
	if os.Getenv("ROOK_CURRENT_NAMESPACE_ONLY") == "true" {
		logger.Infof("watching the current namespace for a ceph cluster CR")
		namespaceToWatch = o.operatorNamespace
	} else {
		logger.Infof("watching all namespaces for ceph cluster CRs")
		namespaceToWatch = v1.NamespaceAll
	}

	// Start the controller-runtime Manager.
	mgrErrorChan := make(chan error)
	go o.startManager(namespaceToWatch, stopContext, mgrErrorChan)

	// Start the operator setting watcher
	go o.clusterController.StartOperatorSettingsWatch(stopChan)

	// Signal handler to stop the operator
	for {
		select {
		case <-signalChan:
			logger.Info("shutdown signal received, exiting...")
			o.cleanup(stopChan)
			return nil
		case err := <-mgrErrorChan:
			logger.Errorf("gave up to run the operator. %v", err)
			o.cleanup(stopChan)
			return err
		}
	}
}

func (o *Operator) startDrivers() error {
	if o.delayedDaemonsStarted {
		return nil
	}

	o.delayedDaemonsStarted = true
	if err := o.updateDrivers(); err != nil {
		o.delayedDaemonsStarted = false // unset because failed to updateDrivers
		return err
	}

	return nil
}

func (o *Operator) updateDrivers() error {
	var err error

	// Skipping CSI driver update since the first cluster hasn't been started yet
	if !o.delayedDaemonsStarted {
		return nil
	}

	if o.operatorNamespace == "" {
		return errors.Errorf("rook operator namespace is not provided. expose it via downward API in the rook operator manifest file using environment variable %s", k8sutil.PodNamespaceEnvVar)
	}

	if opcontroller.FlexDriverEnabled(o.context) {
		rookAgent := agent.New(o.context.Clientset)
		if err := rookAgent.Start(o.operatorNamespace, o.rookImage, o.securityAccount); err != nil {
			return errors.Wrap(err, "error starting agent daemonset")
		}
	}

	serverVersion, err := o.context.Clientset.Discovery().ServerVersion()
	if err != nil {
		return errors.Wrap(err, "error getting server version")
	}

	if serverVersion.Major < csi.KubeMinMajor || serverVersion.Major == csi.KubeMinMajor && serverVersion.Minor < csi.ProvDeploymentSuppVersion {
		logger.Infof("CSI drivers only supported in K8s 1.14 or newer. version=%s", serverVersion.String())
		// disable csi control variables to disable other csi functions
		csi.EnableRBD = false
		csi.EnableCephFS = false
		return nil
	}

	ownerRef, err := getDeploymentOwnerReference(o.context.Clientset, o.operatorNamespace)
	if err != nil {
		logger.Warningf("could not find deployment owner reference to assign to csi drivers. %v", err)
	}
	if ownerRef != nil {
		blockOwnerDeletion := false
		ownerRef.BlockOwnerDeletion = &blockOwnerDeletion
	}

	ownerInfo := k8sutil.NewOwnerInfoWithOwnerRef(ownerRef, o.operatorNamespace)
	// create an empty config map. config map will be filled with data
	// later when clusters have mons
	err = csi.CreateCsiConfigMap(o.operatorNamespace, o.context.Clientset, ownerInfo)
	if err != nil {
		return errors.Wrap(err, "failed creating csi config map")
	}

	go csi.ValidateAndConfigureDrivers(o.context, o.operatorNamespace, o.rookImage, o.securityAccount, serverVersion, ownerInfo)
	return nil
}

// getDeploymentOwnerReference returns an OwnerReference to the rook-ceph-operator deployment
func getDeploymentOwnerReference(clientset kubernetes.Interface, namespace string) (*metav1.OwnerReference, error) {
	ctx := context.TODO()
	var deploymentRef *metav1.OwnerReference
	podName := os.Getenv(k8sutil.PodNameEnvVar)
	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "could not find pod %q to find deployment owner reference", podName)
	}
	for _, podOwner := range pod.OwnerReferences {
		if podOwner.Kind == "ReplicaSet" {
			replicaset, err := clientset.AppsV1().ReplicaSets(namespace).Get(ctx, podOwner.Name, metav1.GetOptions{})
			if err != nil {
				return nil, errors.Wrapf(err, "could not find replicaset %q to find deployment owner reference", podOwner.Name)
			}
			for _, replicasetOwner := range replicaset.OwnerReferences {
				if replicasetOwner.Kind == "Deployment" {
					localreplicasetOwner := replicasetOwner
					deploymentRef = &localreplicasetOwner
				}
			}
		}
	}
	if deploymentRef == nil {
		return nil, errors.New("could not find owner reference for rook-ceph deployment")
	}
	return deploymentRef, nil
}
