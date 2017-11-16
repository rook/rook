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
package rgw

import (
	"fmt"
	"path"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/ceph/client"
	cephrgw "github.com/rook/rook/pkg/ceph/rgw"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
	opmon "github.com/rook/rook/pkg/operator/mon"
	"github.com/rook/rook/pkg/operator/pool"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-rgw")

const (
	appName        = "rook-ceph-rgw"
	keyringName    = "keyring"
	certVolumeName = "rook-rgw-cert"
	certMountPath  = "/etc/rook/private"
	certKeyName    = "cert"
	certFilename   = "rgw-cert.pem"
)

// Start the rgw manager
func (s *ObjectStore) Create(context *clusterd.Context, version string, hostNetwork bool) error {
	return s.createOrUpdate(context, version, hostNetwork, false)
}

func (s *ObjectStore) Update(context *clusterd.Context, version string, hostNetwork bool) error {
	return s.createOrUpdate(context, version, hostNetwork, true)
}

func (s *ObjectStore) createOrUpdate(context *clusterd.Context, version string, hostNetwork, update bool) error {
	// validate the object store settings
	if err := s.validate(context); err != nil {
		return fmt.Errorf("invalid object store %s arguments. %+v", s.Name, err)
	}

	// check if the object store already exists
	exists, err := s.exists(context)
	if err == nil && exists {
		if !update {
			logger.Infof("object store %s exists in namespace %s", s.Name, s.Namespace)
			return nil
		}
		logger.Infof("object store %s exists in namespace %s. checking for updates", s.Name, s.Namespace)
	}

	logger.Infof("creating object store %s in namespace %s", s.Name, s.Namespace)
	err = s.createKeyring(context)
	if err != nil {
		return fmt.Errorf("failed to create rgw keyring. %+v", err)
	}

	// start the service
	serviceIP, err := s.startService(context, hostNetwork)
	if err != nil {
		return fmt.Errorf("failed to start rgw service. %+v", err)
	}

	// create the ceph artifacts for the object store
	objContext := cephrgw.NewContext(context, s.Name, s.Namespace)
	err = cephrgw.CreateObjectStore(objContext, *s.Spec.MetadataPool.ToModel(""), *s.Spec.DataPool.ToModel(""), serviceIP, s.Spec.Gateway.Port)
	if err != nil {
		return fmt.Errorf("failed to create pools. %+v", err)
	}

	if err := s.startRGWPods(context, version, hostNetwork, update); err != nil {
		return fmt.Errorf("failed to start pods. %+v", err)
	}

	logger.Infof("created object store %s", s.Name)
	return nil
}

func (s *ObjectStore) startRGWPods(context *clusterd.Context, version string, hostNetwork, update bool) error {

	// if intended to update, remove the old pods so they can be created with the new spec settings
	if update {
		err := k8sutil.DeleteDeployment(context.Clientset, s.Namespace, s.instanceName())
		if err != nil {
			logger.Warningf(err.Error())
		}
		err = k8sutil.DeleteDaemonset(context.Clientset, s.Namespace, s.instanceName())
		if err != nil {
			logger.Warningf(err.Error())
		}
	}

	// start the deployment or daemonset
	var rgwType string
	var err error
	if s.Spec.Gateway.AllNodes {
		rgwType = "daemonset"
		err = s.startDaemonset(context, version, hostNetwork)
	} else {
		rgwType = "deployment"
		err = s.startDeployment(context, version, s.Spec.Gateway.Instances, hostNetwork)
	}

	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create rgw %s. %+v", rgwType, err)
		}
		logger.Infof("rgw %s already exists", rgwType)
	} else {
		logger.Infof("rgw %s started", rgwType)
	}

	return nil
}

// Delete the object store.
// WARNING: This is a very destructive action that deletes all metadata and data pools.
func (s *ObjectStore) Delete(context *clusterd.Context) error {
	// check if the object store  exists
	exists, err := s.exists(context)
	if err != nil {
		return fmt.Errorf("failed to detect if there is an object store to delete. %+v", err)
	}
	if !exists {
		logger.Infof("Object store %s does not exist in namespace %s", s.Name, s.Namespace)
		return nil
	}

	logger.Infof("Deleting object store %s from namespace %s", s.Name, s.Namespace)

	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the rgw service
	err = context.Clientset.CoreV1().Services(s.Namespace).Delete(s.instanceName(), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete rgw service. %+v", err)
	}

	// Make a best effort to delete the rgw pods
	err = k8sutil.DeleteDeployment(context.Clientset, s.Namespace, s.instanceName())
	if err != nil {
		logger.Warningf(err.Error())
	}
	err = k8sutil.DeleteDaemonset(context.Clientset, s.Namespace, s.instanceName())
	if err != nil {
		logger.Warningf(err.Error())
	}

	// Delete the rgw keyring
	err = context.Clientset.CoreV1().Secrets(s.Namespace).Delete(s.instanceName(), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete rgw secret. %+v", err)
	}

	// Delete the realm and pools
	objContext := cephrgw.NewContext(context, s.Name, s.Namespace)
	err = cephrgw.DeleteObjectStore(objContext)
	if err != nil {
		return fmt.Errorf("failed to delete the realm and pools. %+v", err)
	}

	logger.Infof("Completed deleting object store %s", s.Name)
	return nil
}

// Check if the object store exists depending on either the deployment or the daemonset
func (s *ObjectStore) exists(context *clusterd.Context) (bool, error) {
	_, err := context.Clientset.ExtensionsV1beta1().Deployments(s.Namespace).Get(s.instanceName(), metav1.GetOptions{})
	if err == nil {
		// the deployment was found
		return true, nil
	}
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	_, err = context.Clientset.ExtensionsV1beta1().DaemonSets(s.Namespace).Get(s.instanceName(), metav1.GetOptions{})
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

// Validate the object store arguments
func (s *ObjectStore) validate(context *clusterd.Context) error {
	logger.Debugf("validating object store: %+v", s)
	if s.Name == "" {
		return fmt.Errorf("missing name")
	}
	if s.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if err := s.Spec.MetadataPool.Validate(context, s.Namespace); err != nil {
		return fmt.Errorf("invalid metadata pool spec. %+v", err)
	}
	if err := s.Spec.DataPool.Validate(context, s.Namespace); err != nil {
		return fmt.Errorf("invalid data pool spec. %+v", err)
	}

	return nil
}

func (s *ObjectStore) createKeyring(context *clusterd.Context) error {
	_, err := context.Clientset.CoreV1().Secrets(s.Namespace).Get(s.instanceName(), metav1.GetOptions{})
	if err == nil {
		logger.Infof("the rgw keyring was already generated")
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get rgw secrets. %+v", err)
	}

	// create the keyring
	logger.Infof("generating rgw keyring")
	keyring, err := createKeyring(context, s.Namespace)
	if err != nil {
		return fmt.Errorf("failed to create keyring. %+v", err)
	}

	// store the secrets
	secrets := map[string]string{
		keyringName: keyring,
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: s.instanceName(), Namespace: s.Namespace},
		StringData: secrets,
		Type:       k8sutil.RookType,
	}
	_, err = context.Clientset.CoreV1().Secrets(s.Namespace).Create(secret)
	if err != nil {
		return fmt.Errorf("failed to save rgw secrets. %+v", err)
	}

	return nil
}

func (s *ObjectStore) instanceName() string {
	return InstanceName(s.Name)
}

func InstanceName(name string) string {
	return fmt.Sprintf("%s-%s", appName, name)
}

func ModelToSpec(store model.ObjectStore, namespace string) *ObjectStore {
	return &ObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: store.Name, Namespace: namespace},
		Spec: ObjectStoreSpec{
			MetadataPool: pool.ModelToSpec(store.MetadataConfig),
			DataPool:     pool.ModelToSpec(store.DataConfig),
			Gateway: GatewaySpec{
				Port:              store.Gateway.Port,
				SecurePort:        store.Gateway.SecurePort,
				Instances:         store.Gateway.Instances,
				AllNodes:          store.Gateway.AllNodes,
				SSLCertificateRef: store.Gateway.CertificateRef,
			},
		},
	}
}

func (s *ObjectStore) makeRGWPodSpec(version string, hostNetwork bool) v1.PodTemplateSpec {
	podSpec := v1.PodSpec{
		Containers:    []v1.Container{s.rgwContainer(version)},
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
	s.Spec.Gateway.Placement.ApplyToPodSpec(&podSpec)

	// Set the ssl cert if specified
	if s.Spec.Gateway.SSLCertificateRef != "" {
		certVol := v1.Volume{Name: certVolumeName, VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{
			SecretName: s.Spec.Gateway.SSLCertificateRef,
			Items:      []v1.KeyToPath{{Key: certKeyName, Path: certFilename}},
		}}}
		podSpec.Volumes = append(podSpec.Volumes, certVol)
	}

	s.Spec.Gateway.Placement.ApplyToPodSpec(&podSpec)

	return v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:        s.instanceName(),
			Labels:      s.getLabels(),
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}
}

func (s *ObjectStore) startDeployment(context *clusterd.Context, version string, replicas int32, hostNetwork bool) error {

	deployment := &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.instanceName(),
			Namespace: s.Namespace,
		},
		Spec: extensions.DeploymentSpec{Template: s.makeRGWPodSpec(version, hostNetwork), Replicas: &replicas},
	}
	_, err := context.Clientset.ExtensionsV1beta1().Deployments(s.Namespace).Create(deployment)
	return err
}

func (s *ObjectStore) startDaemonset(context *clusterd.Context, version string, hostNetwork bool) error {

	daemonset := &extensions.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.instanceName(),
			Namespace: s.Namespace,
		},
		Spec: extensions.DaemonSetSpec{
			Template: s.makeRGWPodSpec(version, hostNetwork),
		},
	}

	_, err := context.Clientset.ExtensionsV1beta1().DaemonSets(s.Namespace).Create(daemonset)
	return err
}

func (s *ObjectStore) rgwContainer(version string) v1.Container {

	container := v1.Container{
		Args: []string{
			"rgw",
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
			fmt.Sprintf("--rgw-name=%s", s.Name),
			fmt.Sprintf("--rgw-host=%s", s.instanceName()),
			fmt.Sprintf("--rgw-port=%d", s.Spec.Gateway.Port),
			fmt.Sprintf("--rgw-secure-port=%d", s.Spec.Gateway.SecurePort),
		},
		Name:  s.instanceName(),
		Image: k8sutil.MakeRookImage(version),
		VolumeMounts: []v1.VolumeMount{
			{Name: k8sutil.DataDirVolume, MountPath: k8sutil.DataDir},
			k8sutil.ConfigOverrideMount(),
		},
		Env: []v1.EnvVar{
			{Name: "ROOK_RGW_KEYRING", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: s.instanceName()}, Key: keyringName}}},
			k8sutil.PodIPEnvVar(k8sutil.PrivateIPEnvVar),
			k8sutil.PodIPEnvVar(k8sutil.PublicIPEnvVar),
			opmon.ClusterNameEnvVar(s.Namespace),
			opmon.EndpointEnvVar(),
			opmon.SecretEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
		Resources: s.Spec.Gateway.Resources,
	}

	if s.Spec.Gateway.SSLCertificateRef != "" {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certMountPath, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)

		// Pass the flag for using the ssl cert
		path := path.Join(certMountPath, certFilename)
		container.Args = append(container.Args, fmt.Sprintf("--rgw-cert=%s", path))
	}

	return container
}

func (s *ObjectStore) startService(context *clusterd.Context, hostNetwork bool) (string, error) {
	labels := s.getLabels()
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.instanceName(),
			Namespace: s.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
		},
	}
	if hostNetwork {
		svc.Spec.ClusterIP = v1.ClusterIPNone
	}

	addPort(svc, "http", s.Spec.Gateway.Port)
	addPort(svc, "https", s.Spec.Gateway.SecurePort)

	svc, err := context.Clientset.CoreV1().Services(s.Namespace).Create(svc)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return "", fmt.Errorf("failed to create rgw service. %+v", err)
		}
		logger.Infof("Gateway service already running")
		return "", nil
	}

	logger.Infof("Gateway service running at %s:%d", svc.Spec.ClusterIP, s.Spec.Gateway.Port)
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

func (s *ObjectStore) getLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: s.Namespace,
		"rook_object_store": s.Name,
	}
}

// create a keyring for the rgw client with a limited set of privileges
func createKeyring(context *clusterd.Context, clusterName string) (string, error) {
	username := "client.radosgw.gateway"
	access := []string{"osd", "allow rwx", "mon", "allow rw"}

	// get-or-create-key for the user account
	key, err := client.AuthGetOrCreateKey(context, clusterName, username, access)
	if err != nil {
		return "", fmt.Errorf("failed to get or create auth key for %s. %+v", username, err)
	}

	return key, err
}
