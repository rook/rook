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

// Package to manage a Minio object store.
package minio

import (
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	miniov1alpha1 "github.com/rook/rook/pkg/apis/minio.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/apps/v1beta2"
	"k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

// TODO: A lot of these constants are specific to the KubeCon demo. Let's
// revisit these and determine what should be specified in the resource spec.
const (
	customResourceName       = "objectstore"
	customResourceNamePlural = "objectstores"
	minioCtrName             = "minio"
	minioLabel               = "minio"
	minioPVCName             = "minio-pvc"
	minioVolumeName          = "data"
	objectStoreDataDir       = "/data"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "minio-op-object")

// ObjectStoreResource represents the object store custom resource
var ObjectStoreResource = opkit.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   miniov1alpha1.CustomResourceGroup,
	Version: miniov1alpha1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(miniov1alpha1.ObjectStore{}).Name(),
}

// MinioController represents a controller object for object store custom resources
type MinioController struct {
	context   *clusterd.Context
	rookImage string
}

// NewMinioController create controller for watching object store custom resources created
func NewMinioController(context *clusterd.Context, rookImage string) *MinioController {
	return &MinioController{
		context:   context,
		rookImage: rookImage,
	}
}

// StartWatch watches for instances of ObjectStore custom resources and acts on them
func (c *MinioController) StartWatch(namespace string, stopCh chan struct{}) error {
	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching object store resources in namespace %s", namespace)
	watcher := opkit.NewWatcher(ObjectStoreResource, namespace, resourceHandlerFuncs, c.context.RookClientset.MinioV1alpha1().RESTClient())
	go watcher.Watch(&miniov1alpha1.ObjectStore{}, stopCh)

	return nil
}

func (c *MinioController) makeMinioHeadlessService(name, namespace string, spec miniov1alpha1.ObjectStoreSpec, ownerRef meta_v1.OwnerReference) (*v1.Service, error) {
	coreV1Client := c.context.Clientset.CoreV1()

	svc, err := coreV1Client.Services(namespace).Create(&v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{k8sutil.AppAttr: minioLabel},
		},
		Spec: v1.ServiceSpec{
			Selector:  map[string]string{k8sutil.AppAttr: minioLabel},
			Ports:     []v1.ServicePort{{Port: spec.Port}},
			ClusterIP: v1.ClusterIPNone,
		},
	})
	k8sutil.SetOwnerRef(c.context.Clientset, namespace, &svc.ObjectMeta, &ownerRef)

	return svc, err
}

func (c *MinioController) buildMinioCtrArgs(statefulSetPrefix, headlessServiceName, namespace string, serverCount int32) []string {
	args := []string{"server"}
	for i := int32(0); i < serverCount; i++ {
		serverAddress := fmt.Sprintf("http://%s-%d.%s.%s%s", statefulSetPrefix, i, headlessServiceName, namespace, objectStoreDataDir)
		args = append(args, serverAddress)
	}

	logger.Infof("Building Minio container args: %v", args)
	return args
}

func (c *MinioController) makeMinioPodSpec(name, namespace string, ctrName string, ctrImage string, port int32, envVars map[string]string, numServers int32) v1.PodTemplateSpec {
	var env []v1.EnvVar
	for k, v := range envVars {
		env = append(env, v1.EnvVar{Name: k, Value: v})
	}

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{k8sutil.AppAttr: minioLabel},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:    ctrName,
					Image:   ctrImage,
					Env:     env,
					Command: []string{"/usr/bin/minio"},
					Ports:   []v1.ContainerPort{{ContainerPort: port}},
					Args:    c.buildMinioCtrArgs(name, name, namespace, numServers),
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      minioVolumeName,
							MountPath: objectStoreDataDir,
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: minioVolumeName,
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: minioPVCName,
						},
					},
				},
			},
		},
	}

	return podSpec
}

func (c *MinioController) getAccessCredentials(secretName, namespace string) (string, string, error) {
	coreV1Client := c.context.Clientset.CoreV1()
	var getOpts meta_v1.GetOptions
	val, err := coreV1Client.Secrets(namespace).Get(secretName, getOpts)
	if err != nil {
		logger.Errorf("Unable to get secret with name=%s in namespace=%s: %v", secretName, namespace, err)
		return "", "", err
	}

	return string(val.Data["username"]), string(val.Data["password"]), nil
}

func validateObjectStoreSpec(spec miniov1alpha1.ObjectStoreSpec) error {
	// Verify node count.
	count := spec.Storage.NodeCount
	if count < 4 || count%2 != 0 {
		return fmt.Errorf("Node count must be greater than 3 and even.")
	}

	// Verify sane port.
	if spec.Port < 1024 {
		return fmt.Errorf("Invalid port %d", spec.Port)
	}

	return nil
}

func (c *MinioController) makeMinioStatefulSet(name, namespace string, spec miniov1alpha1.ObjectStoreSpec, ownerRef meta_v1.OwnerReference) (*v1beta2.StatefulSet, error) {
	appsClient := c.context.Clientset.AppsV1beta2()

	accessKey, secretKey, err := c.getAccessCredentials(spec.Credentials.Name, spec.Credentials.Namespace)
	if err != nil {
		return nil, err
	}

	envVars := map[string]string{
		"MINIO_ACCESS_KEY": accessKey,
		"MINIO_SECRET_KEY": secretKey,
	}

	podSpec := c.makeMinioPodSpec(name, namespace, minioCtrName, c.rookImage, spec.Port, envVars, int32(spec.Storage.NodeCount))

	nodeCount := int32(spec.Storage.NodeCount)
	ss := v1beta2.StatefulSet{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{k8sutil.AppAttr: minioLabel},
		},
		Spec: v1beta2.StatefulSetSpec{
			Replicas: &nodeCount,
			Selector: &meta_v1.LabelSelector{
				MatchLabels: map[string]string{k8sutil.AppAttr: minioLabel},
			},
			Template: podSpec,
			VolumeClaimTemplates: []v1.PersistentVolumeClaim{
				{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      minioVolumeName,
						Namespace: namespace,
					},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceStorage: resource.MustParse(spec.StorageSize),
							},
						},
					},
				},
			},
			ServiceName: name,
			// TODO: liveness probe
		},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, namespace, &ss.ObjectMeta, &ownerRef)

	return appsClient.StatefulSets(namespace).Create(&ss)
}

func (c *MinioController) onAdd(obj interface{}) {
	objectstore := obj.(*miniov1alpha1.ObjectStore).DeepCopy()

	ownerRef := meta_v1.OwnerReference{
		APIVersion: ObjectStoreResource.Version,
		Kind:       ObjectStoreResource.Kind,
		Name:       objectstore.Namespace,
		UID:        types.UID(objectstore.ObjectMeta.UID),
	}

	// Validate object store config.
	err := validateObjectStoreSpec(objectstore.Spec)
	if err != nil {
		logger.Errorf("failed to validate object store config")
		return
	}

	// Create the headless service.
	logger.Infof("Creating Minio headless service %s in namespace %s.", objectstore.Name, objectstore.Namespace)
	_, err = c.makeMinioHeadlessService(objectstore.Name, objectstore.Namespace, objectstore.Spec, ownerRef)
	if err != nil {
		logger.Errorf("failed to create minio headless service: %v", err)
		return
	}
	logger.Infof("Finished creating Minio headless service %s in namespace %s.", objectstore.Name, objectstore.Namespace)

	// Create the stateful set.
	logger.Infof("Creating Minio stateful set %s.", objectstore.Name)
	_, err = c.makeMinioStatefulSet(objectstore.Name, objectstore.Namespace, objectstore.Spec, ownerRef)
	if err != nil {
		logger.Errorf("failed to create minio stateful set: %v", err)
		return
	}
	logger.Infof("Finished creating Minio stateful set %s in namespace %s.", objectstore.Name, objectstore.Namespace)
}

func (c *MinioController) onUpdate(oldObj, newObj interface{}) {
	oldStore := oldObj.(*miniov1alpha1.ObjectStore).DeepCopy()
	newStore := newObj.(*miniov1alpha1.ObjectStore).DeepCopy()

	_ = oldStore
	_ = newStore

	logger.Infof("Received update on object store %s in namespace %s. This is currently unsupported.", oldStore.Name, oldStore.Namespace)
}

func (c *MinioController) onDelete(obj interface{}) {
	objectstore := obj.(*miniov1alpha1.ObjectStore).DeepCopy()
	logger.Infof("Delete Minio object store %s", objectstore.Name)

	// Cleanup is handled by the owner references set in 'onAdd' and the k8s garbage collector.
}
