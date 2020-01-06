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
	nfsv1alpha1 "github.com/rook/rook/pkg/apis/nfs.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"
)

const (
	customResourceName       = "nfsserver"
	customResourceNamePlural = "nfsservers"
	NFSConfigMapPath         = "/nfs-ganesha/config"
	nfsPort                  = 2049
	rpcPort                  = 111
	noneMode                 = "none"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "nfs-operator")

// NFSResource represents the nfs export custom resource
var NFSResource = k8sutil.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   nfsv1alpha1.CustomResourceGroup,
	Version: nfsv1alpha1.Version,
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
	go k8sutil.WatchCR(NFSResource, namespace, resourceHandlerFuncs, c.context.RookClientset.NfsV1alpha1().RESTClient(), &nfsv1alpha1.NFSServer{}, stopCh)

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
		name:      c.Name,
		context:   context,
		namespace: c.Namespace,
		spec:      c.Spec,
		ownerRef:  nfsOwnerRef(c.Name, string(c.UID)),
	}
}

func nfsOwnerRef(name, nfsServerID string) metav1.OwnerReference {
	blockOwner := true
	return metav1.OwnerReference{
		APIVersion:         fmt.Sprintf("%s/%s", NFSResource.Group, NFSResource.Version),
		Kind:               NFSResource.Kind,
		Name:               name,
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

func createAppLabels(nfsServer *nfsServer) map[string]string {
	return map[string]string{
		k8sutil.AppAttr: nfsServer.name,
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

func (c *Controller) createNFSService(nfsServer *nfsServer) (*v1.Service, error) {
	// This service is meant to be used by clients to access NFS.
	nfsService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            nfsServer.name,
			Namespace:       nfsServer.namespace,
			OwnerReferences: []metav1.OwnerReference{nfsServer.ownerRef},
			Labels:          createAppLabels(nfsServer),
		},
		Spec: v1.ServiceSpec{
			Selector: createAppLabels(nfsServer),
			Type:     v1.ServiceTypeClusterIP,
			Ports:    createServicePorts(),
		},
	}

	svc, err := c.context.Clientset.CoreV1().Services(nfsServer.namespace).Create(nfsService)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, err
		}
		logger.Infof("nfs service %s already exists in namespace %s", nfsService.Name, nfsService.Namespace)
	} else {
		logger.Infof("nfs service %s started in namespace %s", nfsService.Name, nfsService.Namespace)
	}

	return svc, nil
}

func createCephNFSExport(id int, path string, access string, squash string) string {
	var accessType string
	// validateNFSServerSpec guarantees `access` will be one of these values at this point
	switch s.ToLower(access) {
	case "readwrite":
		accessType = "RW"
	case "readonly":
		accessType = "RO"
	case noneMode:
		accessType = "None"
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
	Squash = ` + s.ToLower(squash) + `;
	FSAL {
		Name = VFS;
	}
}`

	return nfsGaneshaConfig
}

func createCephNFSConfig(spec *nfsv1alpha1.NFSServerSpec) string {
	serverConfig := getServerConfig(spec.Exports)

	exportsList := make([]string, 0)
	id := 10
	for claimName, claimConfig := range serverConfig {
		exportsList = append(exportsList, createCephNFSExport(id, claimName, claimConfig["accessMode"], claimConfig["squash"]))
		id++
	}

	// fsid_device parameter is important as in case of an overlayfs there is a chance that the fsid of the mounted share is same as that of the fsid of "/"
	// so setting this to true uses device number as fsid
	// related issue https://github.com/nfs-ganesha/nfs-ganesha/issues/140
	exportsList = append(exportsList, `NFS_Core_Param
{
	fsid_device = true;
}`)
	nfsGaneshaConfig := s.Join(exportsList, "\n")

	return nfsGaneshaConfig
}

func (c *Controller) createNFSConfigMap(nfsServer *nfsServer) error {
	nfsGaneshaConfig := createCephNFSConfig(&nfsServer.spec)

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            nfsServer.name,
			Namespace:       nfsServer.namespace,
			OwnerReferences: []metav1.OwnerReference{nfsServer.ownerRef},
			Labels:          createAppLabels(nfsServer),
		},
		Data: map[string]string{
			nfsServer.name: nfsGaneshaConfig,
		},
	}
	_, err := c.context.Clientset.CoreV1().ConfigMaps(nfsServer.namespace).Create(configMap)
	if err != nil {
		return err
	}

	return nil
}

func getPVCMount(spec *nfsv1alpha1.NFSServerSpec) map[string]string {
	pvcMounts := map[string]string{}
	for _, export := range spec.Exports {
		claimName := export.PersistentVolumeClaim.ClaimName
		if claimName != "" {
			pvcMounts[export.Name] = export.PersistentVolumeClaim.ClaimName
		}
	}
	return pvcMounts
}

func createPVCSpecList(nfsServer *nfsServer) []v1.Volume {
	pvcSpecList := make([]v1.Volume, 0)
	pvcNameList := getPVCMount(&nfsServer.spec)
	for shareName, claimName := range pvcNameList {
		pvcSpecList = append(pvcSpecList, v1.Volume{
			Name: shareName,
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
				Key:  nfsServer.name,
				Path: nfsServer.name,
			},
		},
	}
	configMapSrc.Name = nfsServer.name
	configMapVol := v1.Volume{
		Name: nfsServer.name,
		VolumeSource: v1.VolumeSource{
			ConfigMap: configMapSrc,
		},
	}
	pvcSpecList = append(pvcSpecList, configMapVol)

	return pvcSpecList
}

func createVolumeMountList(nfsServer *nfsServer) []v1.VolumeMount {
	volumeMountList := make([]v1.VolumeMount, 0)
	pvcMount := getPVCMount(&nfsServer.spec)
	for shareName, claimName := range pvcMount {
		volumeMountList = append(volumeMountList, v1.VolumeMount{
			Name:      shareName,
			MountPath: "/" + claimName,
		})
	}

	configMapVolMount := v1.VolumeMount{
		Name:      nfsServer.name,
		MountPath: NFSConfigMapPath,
	}
	volumeMountList = append(volumeMountList, configMapVolMount)

	return volumeMountList
}

func (c *Controller) createNfsPodSpec(nfsServer *nfsServer, service *v1.Service) v1.PodTemplateSpec {
	nfsPodSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nfsServer.name,
			Namespace: nfsServer.namespace,
			Labels:    createAppLabels(nfsServer),
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					ImagePullPolicy: "IfNotPresent",
					Name:            nfsServer.name,
					Image:           c.containerImage,
					Args:            []string{"nfs", "server", "--ganeshaConfigPath=" + NFSConfigMapPath + "/" + nfsServer.name},
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
					VolumeMounts: createVolumeMountList(nfsServer),
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
			Volumes: createPVCSpecList(nfsServer),
		},
	}

	return nfsPodSpec
}

func (c *Controller) createNfsStatefulSet(nfsServer *nfsServer, replicas int32, service *v1.Service) error {
	appsClient := c.context.Clientset.AppsV1()

	nfsPodSpec := c.createNfsPodSpec(nfsServer, service)

	statefulSet := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            nfsServer.name,
			Namespace:       nfsServer.namespace,
			Labels:          createAppLabels(nfsServer),
			OwnerReferences: []metav1.OwnerReference{nfsServer.ownerRef},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: createAppLabels(nfsServer),
			},
			Template:    nfsPodSpec,
			ServiceName: nfsServer.name,
		},
	}
	nfsServer.spec.Annotations.ApplyToObjectMeta(&statefulSet.Spec.Template.ObjectMeta)
	nfsServer.spec.Annotations.ApplyToObjectMeta(&statefulSet.ObjectMeta)

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

	logger.Infof("validating nfs server spec in namespace %s", nfsServer.namespace)
	if err := validateNFSServerSpec(nfsServer.spec); err != nil {
		logger.Errorf("Invalid NFS Server spec: %+v", err)
		return
	}

	logger.Infof("creating nfs server service in namespace %s", nfsServer.namespace)
	svc, err := c.createNFSService(nfsServer)
	if err != nil {
		logger.Errorf("Unable to create NFS service %+v", err)
	}

	logger.Infof("creating nfs server configuration in namespace %s", nfsServer.namespace)
	if err := c.createNFSConfigMap(nfsServer); err != nil {
		logger.Errorf("Unable to create NFS ConfigMap %+v", err)
	}

	logger.Infof("creating nfs server stateful set in namespace %s", nfsServer.namespace)
	if err := c.createNfsStatefulSet(nfsServer, int32(nfsServer.spec.Replicas), svc); err != nil {
		logger.Errorf("Unable to create NFS stateful set %+v", err)
	}
}

func (c *Controller) onUpdate(oldObj, newObj interface{}) {
	oldNfsServ := oldObj.(*nfsv1alpha1.NFSServer).DeepCopy()

	logger.Infof("Received update on NFS server %s in namespace %s. This is currently unsupported.", oldNfsServ.Name, oldNfsServ.Namespace)
}

func (c *Controller) onDelete(obj interface{}) {
	cluster, ok := obj.(*nfsv1alpha1.NFSServer)
	if !ok {
		return
	}
	cluster = cluster.DeepCopy()
	logger.Infof("cluster %s deleted from namespace %s", cluster.Name, cluster.Namespace)
}

func validateNFSServerSpec(spec nfsv1alpha1.NFSServerSpec) error {
	serverConfig := spec.Exports
	for _, export := range serverConfig {
		if err := validateAccessMode(export.Server.AccessMode); err != nil {
			return err
		}
		if err := validateSquashMode(export.Server.Squash); err != nil {
			return err
		}
	}
	return nil
}

func validateAccessMode(mode string) error {
	switch s.ToLower(mode) {
	case "readonly":
	case "readwrite":
	case noneMode:
	default:
		return fmt.Errorf("Invalid value (%s) for accessMode, valid values are (ReadOnly, ReadWrite, none)", mode)
	}
	return nil
}

func validateSquashMode(mode string) error {
	switch s.ToLower(mode) {
	case "rootid":
	case "root":
	case "all":
	case noneMode:
	default:
		return fmt.Errorf("Invalid value (%s) for squash, valid values are (none, rootId, root, all)", mode)
	}
	return nil
}
