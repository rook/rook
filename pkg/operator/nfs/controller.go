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

// Package nfs to manage an NFS export.
package nfs

import (
	goerrors "errors"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	nfsv1alpha1 "github.com/rook/rook/pkg/apis/nfs.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/apps/v1beta2"
	"k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"
)

const (
	customResourceName       = "nfsserver"
	customResourceNamePlural = "nfsservers"
	appName                  = "rook-nfs"
	nfsVolumeName            = "nfsConfigData"
	nfsConfigDataDir         = "/nfs-ganesha"
	nfsPort                  = 2049
	rpcPort                  = 111
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "nfs-operator")

// NFSResource represents the nfs export custom resource
var NFSResource = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   nfsv1alpha1.CustomResourceGroup,
	Version: nfsv1alpha1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(nfsv1alpha1.NFSServer{}).Name(),
}

// Controller represents a controller object for nfs server custom resources
type Controller struct {
	context        *clusterd.Context
	containerImage string
}

// NewController create controller for watching nfsserver custom resources created
func NewController(context *clusterd.Context, containerImage string) *Controller {
	return &Controller{
		context:        context,
		containerImage: containerImage,
	}
}

// StartWatch watches for instances of nfsserver custom resources and acts on them
func (c *Controller) StartWatch(namespace string, stopCh chan struct{}) error {
	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching nfs server resources in namespace %s", namespace)
	watcher := opkit.NewWatcher(NFSResource, namespace, resourceHandlerFuncs, c.context.RookClientset.NfsV1alpha1().RESTClient())
	go watcher.Watch(&nfsv1alpha1.NFSServer{}, stopCh)

	return nil
}

type nfsServer struct {
	name      string
	context   *clusterd.Context
	namespace string
	spec      nfsv1alpha1.NFSServerSpec
	ownerRef  metav1.OwnerReference
}

func newNfsServer(c *nfsv1alpha1.NFSServer, context *clusterd.Context) *nfsServer {
	return &nfsServer{
		name:      appName,
		context:   context,
		namespace: c.Namespace,
		spec:      c.Spec,
		ownerRef:  nfsOwnerRef(c.Namespace, string(c.UID)),
	}
}

func nfsOwnerRef(namespace, nfsServerID string) metav1.OwnerReference {
	blockOwner := true
	return metav1.OwnerReference{
		APIVersion:         NFSResource.Version,
		Kind:               NFSResource.Kind,
		Name:               namespace,
		UID:                types.UID(nfsServerID),
		BlockOwnerDeletion: &blockOwner,
	}
}

func getServerConfig(exports []nfsv1alpha1.ExportsSpec) map[string]string {
	configOpt := make(map[string]string)

	for _, export := range exports {
		for _, server := range export.Server {
			configOpt["accessMode"] = server.AccessMode
			configOpt["squash"] = server.Squash
		}
	}

	return configOpt
}

func getClientConfig(exports []nfsv1alpha1.ExportsSpec) map[string]string {
	configOpt := make(map[string]string)

	for _, export := range exports {
		for _, server := range export.Server {
			for _, client := range server.AllowedClients {
				configOpt["accessMode"] = client.AccessMode
				configOpt["squash"] = client.Squash
			}
		}
	}

	return configOpt
}

func validateServerConfig(spec *nfsv1alpha1.NFSServerSpec) error {
	serverConfig := getServerConfig(spec.Exports)
	clientConfig := getClientConfig(spec.Exports)

	accessMode := serverConfig["accessMode"]
	squash := serverConfig["squash"]
	clientAccessMode := clientConfig["accessMode"]
	clientSquash := clientConfig["squash"]

	if err := checkAccessMode(accessMode); err != nil {
		return err
	}

	if err := checkAccessMode(clientAccessMode); err != nil {
		return err
	}

	if err := checkSquash(squash); err != nil {
		return err
	}

	if err := checkSquash(clientSquash); err != nil {
		return err
	}

	return nil
}

func checkAccessMode(accessMode string) error {
	// TODO: Add code to check access mode for "ReadWrite", "ReadOnly" and "none"
	// Current focusing on MVP which will have only ReadWrite for the time being
	if accessMode != "ReadWrite" {
		return goerrors.New(`Currently "ReadWrite" is the only supported access mode`)
	}

	return nil
}

func checkSquash(squash string) error {
	// TODO: Add code to check squash mode for "none", "rootid", "root" and "all"
	// Current focusing on MVP which will have only "root" as squash value for the time being
	if squash != "root" {
		return goerrors.New(`Currently "root" is the only supported squash option`)
	}

	return nil
}

func createAppLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr: appName,
	}
}

func createServicePorts() []v1.ServicePort {
	return []v1.ServicePort{
		{
			Name:       "nfs",
			Port:       int32(nfsPort),
			TargetPort: intstr.FromInt(int(nfsPort)),
		},
		{
			Name:       "rpc",
			Port:       int32(rpcPort),
			TargetPort: intstr.FromInt(int(rpcPort)),
		},
	}
}

func (c *Controller) createNFSService(nfsServer *nfsServer) error {
	// This service is meant to be used by clients to access NFS.
	nfsService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            nfsServer.name,
			Namespace:       nfsServer.namespace,
			OwnerReferences: []metav1.OwnerReference{nfsServer.ownerRef},
			Labels:          createAppLabels(),
		},
		Spec: v1.ServiceSpec{
			Selector: createAppLabels(),
			Type:     v1.ServiceTypeClusterIP,
			Ports:    createServicePorts(),
		},
	}

	if _, err := c.context.Clientset.CoreV1().Services(nfsServer.namespace).Create(nfsService); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("nfs service %s already exists in namespace %s", nfsService.Name, nfsService.Namespace)
	} else {
		logger.Infof("nfs service %s started in namespace %s", nfsService.Name, nfsService.Namespace)
	}

	return nil
}

func createPVCSpec(pvcName string, claimName string) v1.Volume {
	pvcSpec := v1.Volume{
		Name: pvcName,
		VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: claimName,
			},
		},
	}

	return pvcSpec
}

func createPVCList(spec *nfsv1alpha1.NFSServerSpec) []v1.Volume {
	exports := spec.Exports

	pvcList := make([]v1.Volume, 1)
	for _, export := range exports {
		pvcName := export.Name
		claimName := export.PersistentVolumeClaim.ClaimName
		pvcList = append(pvcList, createPVCSpec(pvcName, claimName))
	}

	return pvcList
}

func (c *Controller) createNfsPodSpec(nfsServer *nfsServer) v1.PodTemplateSpec {
	nfsPodSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nfsServer.name,
			Namespace: nfsServer.namespace,
			Labels:    createAppLabels(),
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:    nfsServer.name,
					Image:   c.containerImage,
					Command: []string{"/nfs-ganesha/start"},
					Ports: []v1.ContainerPort{
						{
							Name:          "nfsPort",
							ContainerPort: int32(nfsPort),
						},
						{
							Name:          "rpcPort",
							ContainerPort: int32(rpcPort),
						},
					},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      nfsVolumeName,
							MountPath: nfsConfigDataDir,
						},
					},
					SecurityContext: &v1.SecurityContext{
						Capabilities: &v1.Capabilities{
							Add: []v1.Capability{
								"CAP_SYS_ADMIN",
								"DAC_READ_SEARCH",
							},
						},
					},
				},
			},
			Volumes: createPVCList(&nfsServer.spec),
		},
	}

	return nfsPodSpec
}

func (c *Controller) createNfsStatefulSet(nfsServer *nfsServer, replicas int32) error {
	appsClient := c.context.Clientset.AppsV1beta2()

	nfsPodSpec := c.createNfsPodSpec(nfsServer)

	statefulSet := v1beta2.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            nfsServer.name,
			Namespace:       nfsServer.namespace,
			Labels:          createAppLabels(),
			OwnerReferences: []metav1.OwnerReference{nfsServer.ownerRef},
		},
		Spec: v1beta2.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: createAppLabels(),
			},
			Template:    nfsPodSpec,
			ServiceName: nfsServer.name,
		},
	}

	if _, err := appsClient.StatefulSets(nfsServer.namespace).Create(&statefulSet); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("stateful set %s already exists in namespace %s", statefulSet.Name, statefulSet.Namespace)
	} else {
		logger.Infof("stateful set %s created in namespace %s", statefulSet.Name, statefulSet.Namespace)
	}

	return nil
}

func (c *Controller) onAdd(obj interface{}) {
	nfsObj := obj.(*nfsv1alpha1.NFSServer).DeepCopy()

	nfsServer := newNfsServer(nfsObj, c.context)

	logger.Infof("new NFS server %s added to namespace %s", nfsObj.Name, nfsServer.namespace)

	if err := validateServerConfig(&nfsServer.spec); err != nil {
		logger.Errorf("Invalid NFS server configuration spec %+v", err)
	}

	if err := c.createNFSService(nfsServer); err != nil {
		logger.Errorf("Unable to create NFS service %+v", err)
	}

	if err := c.createNfsStatefulSet(nfsServer, int32(nfsServer.spec.Replicas)); err != nil {
		logger.Errorf("Unable to create NFS stateful set %+v", err)
	}
}

func (c *Controller) onUpdate(oldObj, newObj interface{}) {
	oldNfsServ := oldObj.(*nfsv1alpha1.NFSServer).DeepCopy()
	newNfsServ := newObj.(*nfsv1alpha1.NFSServer).DeepCopy()

	_ = oldNfsServ
	_ = newNfsServ

	logger.Infof("Received update on NFS server %s in namespace %s. This is currently unsupported.", oldNfsServ.Name, oldNfsServ.Namespace)
}

func (c *Controller) onDelete(obj interface{}) {
	cluster := obj.(*nfsv1alpha1.NFSServer).DeepCopy()
	logger.Infof("cluster %s deleted from namespace %s", cluster.Name, cluster.Namespace)
}
