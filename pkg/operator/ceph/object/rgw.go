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

// Package rgw for the Ceph object store.
package object

import (
	"fmt"
	"path"

	cephv1alpha1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephrgw "github.com/rook/rook/pkg/daemon/ceph/rgw"
	opmon "github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	appName        = "rook-ceph-rgw"
	keyringName    = "keyring"
	certVolumeName = "rook-rgw-cert"
	certMountPath  = "/etc/rook/private"
	certKeyName    = "cert"
	certFilename   = "rgw-cert.pem"
)

// Start the rgw manager
func CreateStore(context *clusterd.Context, store cephv1alpha1.ObjectStore, version string, hostNetwork bool, ownerRefs []metav1.OwnerReference) error {
	return createOrUpdate(context, store, version, hostNetwork, false, ownerRefs)
}

func UpdateStore(context *clusterd.Context, store cephv1alpha1.ObjectStore, version string, hostNetwork bool, ownerRefs []metav1.OwnerReference) error {
	return createOrUpdate(context, store, version, hostNetwork, true, ownerRefs)
}

func createOrUpdate(context *clusterd.Context, store cephv1alpha1.ObjectStore, version string, hostNetwork, update bool, ownerRefs []metav1.OwnerReference) error {
	// validate the object store settings
	if err := validateStore(context, store); err != nil {
		return fmt.Errorf("invalid object store %s arguments. %+v", store.Name, err)
	}

	// check if the object store already exists
	exists, err := storeExists(context, store)
	if err == nil && exists {
		if !update {
			logger.Infof("object store %s exists in namespace %s", store.Name, store.Namespace)
			return nil
		}
		logger.Infof("object store %s exists in namespace %store. checking for updates", store.Name, store.Namespace)
	}

	logger.Infof("creating object store %s in namespace %s", store.Name, store.Namespace)
	err = createKeyring(context, store, ownerRefs)
	if err != nil {
		return fmt.Errorf("failed to create rgw keyring. %+v", err)
	}

	// start the service
	serviceIP, err := startService(context, store, hostNetwork, ownerRefs)
	if err != nil {
		return fmt.Errorf("failed to start rgw service. %+v", err)
	}

	// create the ceph artifacts for the object store
	objContext := cephrgw.NewContext(context, store.Name, store.Namespace)
	err = cephrgw.CreateObjectStore(objContext, *store.Spec.MetadataPool.ToModel(""), *store.Spec.DataPool.ToModel(""), serviceIP, store.Spec.Gateway.Port)
	if err != nil {
		return fmt.Errorf("failed to create pools. %+v", err)
	}

	if err := startRGWPods(context, store, version, hostNetwork, update, ownerRefs); err != nil {
		return fmt.Errorf("failed to start pods. %+v", err)
	}

	logger.Infof("created object store %s", store.Name)
	return nil
}

func startRGWPods(context *clusterd.Context, store cephv1alpha1.ObjectStore, version string, hostNetwork, update bool, ownerRefs []metav1.OwnerReference) error {

	// if intended to update, remove the old pods so they can be created with the new spec settings
	if update {
		err := k8sutil.DeleteDeployment(context.Clientset, store.Namespace, instanceName(store))
		if err != nil {
			logger.Warningf(err.Error())
		}
		err = k8sutil.DeleteDaemonset(context.Clientset, store.Namespace, instanceName(store))
		if err != nil {
			logger.Warningf(err.Error())
		}
	}

	// start the deployment or daemonset
	var rgwType string
	var err error
	if store.Spec.Gateway.AllNodes {
		rgwType = "daemonset"
		err = startDaemonset(context, store, version, hostNetwork, ownerRefs)
	} else {
		rgwType = "deployment"
		err = startDeployment(context, store, version, store.Spec.Gateway.Instances, hostNetwork, ownerRefs)
	}

	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create rgw %store. %+v", rgwType, err)
		}
		logger.Infof("rgw %s already exists", rgwType)
	} else {
		logger.Infof("rgw %s started", rgwType)
	}

	return nil
}

// Delete the object store.
// WARNING: This is a very destructive action that deletes all metadata and data pools.
func DeleteStore(context *clusterd.Context, store cephv1alpha1.ObjectStore) error {
	// check if the object store  exists
	exists, err := storeExists(context, store)
	if err != nil {
		return fmt.Errorf("failed to detect if there is an object store to delete. %+v", err)
	}
	if !exists {
		logger.Infof("Object store %s does not exist in namespace %s", store.Name, store.Namespace)
		return nil
	}

	logger.Infof("Deleting object store %s from namespace %s", store.Name, store.Namespace)

	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the rgw service
	err = context.Clientset.CoreV1().Services(store.Namespace).Delete(instanceName(store), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete rgw service. %+v", err)
	}

	// Make a best effort to delete the rgw pods
	err = k8sutil.DeleteDeployment(context.Clientset, store.Namespace, instanceName(store))
	if err != nil {
		logger.Warningf(err.Error())
	}
	err = k8sutil.DeleteDaemonset(context.Clientset, store.Namespace, instanceName(store))
	if err != nil {
		logger.Warningf(err.Error())
	}

	// Delete the rgw keyring
	err = context.Clientset.CoreV1().Secrets(store.Namespace).Delete(instanceName(store), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete rgw secret. %+v", err)
	}

	// Delete the realm and pools
	objContext := cephrgw.NewContext(context, store.Name, store.Namespace)
	err = cephrgw.DeleteObjectStore(objContext)
	if err != nil {
		return fmt.Errorf("failed to delete the realm and pools. %+v", err)
	}

	logger.Infof("Completed deleting object store %s", store.Name)
	return nil
}

// Check if the object store exists depending on either the deployment or the daemonset
func storeExists(context *clusterd.Context, store cephv1alpha1.ObjectStore) (bool, error) {
	_, err := context.Clientset.ExtensionsV1beta1().Deployments(store.Namespace).Get(instanceName(store), metav1.GetOptions{})
	if err == nil {
		// the deployment was found
		return true, nil
	}
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	_, err = context.Clientset.ExtensionsV1beta1().DaemonSets(store.Namespace).Get(instanceName(store), metav1.GetOptions{})
	if err == nil {
		//  the daemonset was found
		return true, nil
	}
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	// neither one was found
	return false, nil
}

func createKeyring(context *clusterd.Context, store cephv1alpha1.ObjectStore, ownerRefs []metav1.OwnerReference) error {
	_, err := context.Clientset.CoreV1().Secrets(store.Namespace).Get(instanceName(store), metav1.GetOptions{})
	if err == nil {
		logger.Infof("the rgw keyring was already generated")
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get rgw secrets. %+v", err)
	}

	// create the keyring
	logger.Infof("generating rgw keyring")
	keyring, err := createRGWKeyring(context, store.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create keyring. %+v", err)
	}

	// store the secrets
	secrets := map[string]string{
		keyringName: keyring,
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            instanceName(store),
			Namespace:       store.Namespace,
			OwnerReferences: ownerRefs,
		},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	_, err = context.Clientset.CoreV1().Secrets(store.Namespace).Create(secret)
	if err != nil {
		return fmt.Errorf("failed to save rgw secrets. %+v", err)
	}

	return nil
}

func instanceName(store cephv1alpha1.ObjectStore) string {
	return InstanceName(store.Name)
}

func InstanceName(name string) string {
	return fmt.Sprintf("%s-%s", appName, name)
}

func makeRGWPodSpec(store cephv1alpha1.ObjectStore, version string, hostNetwork bool) v1.PodTemplateSpec {
	podSpec := v1.PodSpec{
		Containers:    []v1.Container{rgwContainer(store, version)},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: []v1.Volume{
			{Name: k8sutil.DataDirVolume, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
			k8sutil.ConfigOverrideVolume(),
		},
		HostNetwork: hostNetwork,
	}
	if hostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}

	// Set the ssl cert if specified
	if store.Spec.Gateway.SSLCertificateRef != "" {
		certVol := v1.Volume{Name: certVolumeName, VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{
			SecretName: store.Spec.Gateway.SSLCertificateRef,
			Items:      []v1.KeyToPath{{Key: certKeyName, Path: certFilename}},
		}}}
		podSpec.Volumes = append(podSpec.Volumes, certVol)
	}

	store.Spec.Gateway.Placement.ApplyToPodSpec(&podSpec)

	return v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:        instanceName(store),
			Labels:      getLabels(store),
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}
}

func startDeployment(context *clusterd.Context, store cephv1alpha1.ObjectStore, version string, replicas int32, hostNetwork bool, ownerRefs []metav1.OwnerReference) error {

	deployment := &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            instanceName(store),
			Namespace:       store.Namespace,
			OwnerReferences: ownerRefs,
		},
		Spec: extensions.DeploymentSpec{Template: makeRGWPodSpec(store, version, hostNetwork), Replicas: &replicas},
	}
	_, err := context.Clientset.ExtensionsV1beta1().Deployments(store.Namespace).Create(deployment)
	return err
}

func startDaemonset(context *clusterd.Context, store cephv1alpha1.ObjectStore, version string, hostNetwork bool, ownerRefs []metav1.OwnerReference) error {

	daemonset := &extensions.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            instanceName(store),
			Namespace:       store.Namespace,
			OwnerReferences: ownerRefs,
		},
		Spec: extensions.DaemonSetSpec{
			UpdateStrategy: extensions.DaemonSetUpdateStrategy{
				Type: extensions.RollingUpdateDaemonSetStrategyType,
			},
			Template: makeRGWPodSpec(store, version, hostNetwork),
		},
	}

	_, err := context.Clientset.ExtensionsV1beta1().DaemonSets(store.Namespace).Create(daemonset)
	return err
}

func rgwContainer(store cephv1alpha1.ObjectStore, version string) v1.Container {

	container := v1.Container{
		Args: []string{
			"ceph",
			"rgw",
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
			fmt.Sprintf("--rgw-name=%s", store.Name),
			fmt.Sprintf("--rgw-port=%d", store.Spec.Gateway.Port),
			fmt.Sprintf("--rgw-secure-port=%d", store.Spec.Gateway.SecurePort),
		},
		Name:  instanceName(store),
		Image: k8sutil.MakeRookImage(version),
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
			k8sutil.ConfigOverrideMount(),
		},
		Env: []v1.EnvVar{
			{Name: "ROOK_RGW_KEYRING", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: instanceName(store)}, Key: keyringName}}},
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
			opmon.ClusterNameEnvVar(store.Namespace),
			opmon.EndpointEnvVar(),
			opmon.SecretEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
		Resources: store.Spec.Gateway.Resources,
	}

	if store.Spec.Gateway.SSLCertificateRef != "" {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certMountPath, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)

		// Pass the flag for using the ssl cert
		path := path.Join(certMountPath, certFilename)
		container.Args = append(container.Args, fmt.Sprintf("--rgw-cert=%s", path))
	}

	return container
}

func startService(context *clusterd.Context, store cephv1alpha1.ObjectStore, hostNetwork bool, ownerRefs []metav1.OwnerReference) (string, error) {
	labels := getLabels(store)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            instanceName(store),
			Namespace:       store.Namespace,
			Labels:          labels,
			OwnerReferences: ownerRefs,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
		},
	}
	if hostNetwork {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	addPort(svc, "http", store.Spec.Gateway.Port)
	addPort(svc, "https", store.Spec.Gateway.SecurePort)

	svc, err := context.Clientset.CoreV1().Services(store.Namespace).Create(svc)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return "", fmt.Errorf("failed to create rgw service. %+v", err)
		}
		logger.Infof("Gateway service already running")
		return "", nil
	}

	logger.Infof("Gateway service running at %s:%d", svc.Spec.ClusterIP, store.Spec.Gateway.Port)
	return svc.Spec.ClusterIP, nil
}

func addPort(service *v1.Service, name string, port int32) {
	if port == 0 {
		return
	}
	service.Spec.Ports = append(service.Spec.Ports, v1.ServicePort{
		Name:       name,
		Port:       port,
		TargetPort: intstr.FromInt(int(port)),
		Protocol:   v1.ProtocolTCP,
	})
}

func getLabels(store cephv1alpha1.ObjectStore) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: store.Namespace,
		"rook_object_store": store.Name,
	}
}

// create a keyring for the rgw client with a limited set of privileges
func createRGWKeyring(context *clusterd.Context, clusterName string) (string, error) {
	username := "client.radosgw.gateway"
	access := []string{"osd", "allow rwx", "mon", "allow rw"}

	// get-or-create-key for the user account
	key, err := client.AuthGetOrCreateKey(context, clusterName, username, access)
	if err != nil {
		return "", fmt.Errorf("failed to get or create auth key for %store. %+v", username, err)
	}

	return key, err
}

// Validate the object store arguments
func validateStore(context *clusterd.Context, s cephv1alpha1.ObjectStore) error {
	if s.Name == "" {
		return fmt.Errorf("missing name")
	}
	if s.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if err := pool.ValidatePoolSpec(context, s.Namespace, &s.Spec.MetadataPool); err != nil {
		return fmt.Errorf("invalid metadata pool spec. %+v", err)
	}
	if err := pool.ValidatePoolSpec(context, s.Namespace, &s.Spec.DataPool); err != nil {
		return fmt.Errorf("invalid data pool spec. %+v", err)
	}

	return nil
}
