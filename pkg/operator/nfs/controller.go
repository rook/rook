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
	"fmt"
	"reflect"
	s "strings"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	nfsv1alpha1 "github.com/rook/rook/pkg/apis/nfs.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/apps/v1beta1"
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
	nfsConfigMapName         = "nfs-ganesha-config"
	nfsConfigMapPath         = "/nfs-ganesha/config"
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

func getServerConfig(exports []nfsv1alpha1.ExportsSpec) map[string]map[string]string {
	claimConfigOpt := make(map[string]map[string]string)
	configOpt := make(map[string]string)

	for _, export := range exports {
		claimName := export.PersistentVolumeClaim.ClaimName
		if claimName != "" {
			configOpt["accessMode"] = export.Server.AccessMode
			configOpt["squash"] = export.Server.Squash
			claimConfigOpt[claimName] = configOpt
		}
	}

	return claimConfigOpt
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

func createGaneshaExport(id int, path string, access string, squash string) string {
	var accessType string
	if access == "ReadWrite" {
		accessType = "RW"
	}
	idStr := fmt.Sprintf("%v", id)
	nfsGaneshaConfig := `
EXPORT {
	Export_Id = ` + idStr + `;
	Path = /` + path + `;
	Pseudo = /` + path + `;
	Protocols = 4;
	Transports = TCP;
	Sectype = sys;
	Access_Type = ` + accessType + `;
	Squash = ` + squash + `;
	FSAL {
		Name = VFS;
	}
}`

	return nfsGaneshaConfig
}

func createGaneshaConfig(spec *nfsv1alpha1.NFSServerSpec) string {
	serverConfig := getServerConfig(spec.Exports)

	exportsList := make([]string, 0)
	id := 10
	for claimName, claimConfig := range serverConfig {
		exportsList = append(exportsList, createGaneshaExport(id, claimName, claimConfig["accessMode"], claimConfig["squash"]))
		id++
	}
	exportsList = append(exportsList, `NFS_Core_Param
{
	fsid_device = true;
}`)
	nfsGaneshaConfig := s.Join(exportsList, "\n")

	return nfsGaneshaConfig
}

func (c *Controller) createNFSConfigMap(nfsServer *nfsServer) error {
	nfsGaneshaConfig := createGaneshaConfig(&nfsServer.spec)

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            nfsConfigMapName,
			Namespace:       nfsServer.namespace,
			OwnerReferences: []metav1.OwnerReference{nfsServer.ownerRef},
			Labels:          createAppLabels(),
		},
		Data: map[string]string{
			nfsConfigMapName: nfsGaneshaConfig,
		},
	}
	_, err := c.context.Clientset.CoreV1().ConfigMaps(nfsServer.namespace).Create(configMap)
	if err != nil {
		return err
	}

	return nil
}

func getPVCNameList(spec *nfsv1alpha1.NFSServerSpec) []string {
	exports := spec.Exports
	pvcNameList := make([]string, 0)
	for _, export := range exports {
		claimName := export.PersistentVolumeClaim.ClaimName
		if claimName != "" {
			pvcNameList = append(pvcNameList, claimName)
		}
	}

	return pvcNameList
}

func createPVCSpecList(spec *nfsv1alpha1.NFSServerSpec) []v1.Volume {
	pvcSpecList := make([]v1.Volume, 0)
	pvcNameList := getPVCNameList(spec)
	for _, claimName := range pvcNameList {
		pvcSpecList = append(pvcSpecList, v1.Volume{
			Name: claimName,
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
	}

	configMapSrc := &v1.ConfigMapVolumeSource{
		Items: []v1.KeyToPath{
			{
				Key:  nfsConfigMapName,
				Path: nfsConfigMapName,
			},
		},
	}
	configMapSrc.Name = nfsConfigMapName
	configMapVol := v1.Volume{
		Name: nfsConfigMapName,
		VolumeSource: v1.VolumeSource{
			ConfigMap: configMapSrc,
		},
	}
	pvcSpecList = append(pvcSpecList, configMapVol)

	return pvcSpecList
}

func createVolumeMountList(spec *nfsv1alpha1.NFSServerSpec) []v1.VolumeMount {
	volumeMountList := make([]v1.VolumeMount, 0)
	pvcNameList := getPVCNameList(spec)
	for _, claimName := range pvcNameList {
		volumeMountList = append(volumeMountList, v1.VolumeMount{
			Name:      claimName,
			MountPath: "/" + claimName,
		})
	}

	configMapVolMount := v1.VolumeMount{
		Name:      nfsConfigMapName,
		MountPath: nfsConfigMapPath,
	}
	volumeMountList = append(volumeMountList, configMapVolMount)

	return volumeMountList
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
					Command: []string{"/start.sh"},
					Ports: []v1.ContainerPort{
						{
							Name:          "nfs-port",
							ContainerPort: int32(nfsPort),
						},
						{
							Name:          "rpc-port",
							ContainerPort: int32(rpcPort),
						},
					},
					VolumeMounts: createVolumeMountList(&nfsServer.spec),
					SecurityContext: &v1.SecurityContext{
						Capabilities: &v1.Capabilities{
							Add: []v1.Capability{
								"SYS_ADMIN",
								"DAC_READ_SEARCH",
							},
						},
					},
				},
			},
			Volumes: createPVCSpecList(&nfsServer.spec),
		},
	}

	return nfsPodSpec
}

func (c *Controller) createNfsStatefulSet(nfsServer *nfsServer, replicas int32) error {
	appsClient := c.context.Clientset.AppsV1beta1()

	nfsPodSpec := c.createNfsPodSpec(nfsServer)

	statefulSet := v1beta1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            nfsServer.name,
			Namespace:       nfsServer.namespace,
			Labels:          createAppLabels(),
			OwnerReferences: []metav1.OwnerReference{nfsServer.ownerRef},
		},
		Spec: v1beta1.StatefulSetSpec{
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

	logger.Infof("creating nfs server service in namespace %s", nfsServer.namespace)
	if err := c.createNFSService(nfsServer); err != nil {
		logger.Errorf("Unable to create NFS service %+v", err)
	}

	logger.Infof("creating nfs server configuration in namespace %s", nfsServer.namespace)
	if err := c.createNFSConfigMap(nfsServer); err != nil {
		logger.Errorf("Unable to create NFS ConfigMap %+v", err)
	}

	logger.Infof("creating nfs server stateful set in namespace %s", nfsServer.namespace)
	if err := c.createNfsStatefulSet(nfsServer, int32(nfsServer.spec.Replicas)); err != nil {
		logger.Errorf("Unable to create NFS stateful set %+v", err)
	}
}

func (c *Controller) onUpdate(oldObj, newObj interface{}) {
	oldNfsServ := oldObj.(*nfsv1alpha1.NFSServer).DeepCopy()

	logger.Infof("Received update on NFS server %s in namespace %s. This is currently unsupported.", oldNfsServ.Name, oldNfsServ.Namespace)
}

func (c *Controller) onDelete(obj interface{}) {
	cluster := obj.(*nfsv1alpha1.NFSServer).DeepCopy()
	logger.Infof("cluster %s deleted from namespace %s", cluster.Name, cluster.Namespace)
}
