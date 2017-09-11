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
func (s *ObjectStore) Create(context *clusterd.Context, version string) error {

	// validate the object store settings
	if err := s.validate(); err != nil {
		return fmt.Errorf("invalid object store %s arguments. %+v", s.Name, err)
	}

	// check if the object store already exists
	exists, err := s.exists(context)
	if err == nil && exists {
		logger.Infof("object store %s already exists in namespace %s ", s.Name, s.Namespace)
		return nil
	}

	logger.Infof("creating object store %s in namespace %s", s.Name, s.Namespace)
	err = s.createKeyring(context)
	if err != nil {
		return fmt.Errorf("failed to create rgw keyring. %+v", err)
	}

	// start the service
	serviceIP, err := s.startService(context)
	if err != nil {
		return fmt.Errorf("failed to start rgw service. %+v", err)
	}

	// create the ceph artifacts for the object store
	objContext := cephrgw.NewContext(context, s.Name, s.Namespace)
	err = s.createObjectStore(objContext, serviceIP)
	if err != nil {
		return fmt.Errorf("failed to create ceph object store. %+v", err)
	}

	// start the deployment
	deployment := s.makeDeployment(version)
	_, err = context.Clientset.ExtensionsV1beta1().Deployments(s.Namespace).Create(deployment)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create rgw deployment. %+v", err)
		}
		logger.Infof("rgw deployment already exists")
	} else {
		logger.Infof("rgw deployment started")
	}

	logger.Infof("created object store %s", s.Name)
	return nil
}

func (s *ObjectStore) createObjectStore(context *cephrgw.Context, serviceIP string) error {
	metadataSpec := s.findPoolConfig(s.Spec.MetadataPoolSpec)
	if metadataSpec == nil {
		return fmt.Errorf("failed to find metadata pool %s config", s.Spec.MetadataPoolSpec)
	}

	dataSpec := s.findPoolConfig(s.Spec.DataPoolSpec)
	if dataSpec == nil {
		return fmt.Errorf("failed to find data pool %s config", s.Spec.DataPoolSpec)
	}

	mModel := model.Pool{}
	dModel := model.Pool{}
	metadataSpec.ToModel(&mModel)
	dataSpec.ToModel(&dModel)

	err := cephrgw.CreateObjectStore(context, mModel, dModel, serviceIP, s.Spec.RGW.Port)
	if err != nil {
		return fmt.Errorf("failed to create pools. %+v", err)
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
		logger.Warning("failed to delete rgw service. %+v", err)
	}

	// Delete the rgw deployment
	err = context.Clientset.ExtensionsV1beta1().Deployments(s.Namespace).Delete(s.instanceName(), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warning("failed to delete rgw deployment. %+v", err)
	}

	// Delete the rgw keyring
	err = context.Clientset.CoreV1().Secrets(s.Namespace).Delete(s.instanceName(), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warning("failed to delete rgw secret. %+v", err)
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

// Check if the object store exists
func (s *ObjectStore) exists(context *clusterd.Context) (bool, error) {
	_, err := context.Clientset.ExtensionsV1beta1().Deployments(s.Namespace).Get(s.instanceName(), metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// Validate the object store arguments
func (s *ObjectStore) validate() error {
	logger.Debugf("validating object store: %+v", s)
	if s.Name == "" {
		return fmt.Errorf("missing name")
	}
	if s.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	for _, pool := range s.Spec.PoolSpecs {
		if err := pool.PoolSpec.Validate(); err != nil {
			return fmt.Errorf("invalid pool spec %s. %+v", pool.Name, err)
		}
	}
	if s.findPoolConfig(s.Spec.MetadataPoolSpec) == nil {
		return fmt.Errorf("metadata pool %s not found", s.Spec.MetadataPoolSpec)
	}
	if s.findPoolConfig(s.Spec.DataPoolSpec) == nil {
		return fmt.Errorf("data pool %s not found", s.Spec.DataPoolSpec)
	}

	return nil
}

func (s *ObjectStore) findPoolConfig(name string) *pool.PoolSpec {
	for _, p := range s.Spec.PoolSpecs {
		if p.Name == name {
			return &p.PoolSpec
		}
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
	keyring, err := cephrgw.CreateKeyring(context, s.Namespace)
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
	metaName := "meta"
	dataName := "data"
	return &ObjectStore{
		ObjectMeta: metav1.ObjectMeta{Name: store.Name, Namespace: namespace},
		Spec: ObjectStoreSpec{
			MetadataPoolSpec: metaName,
			DataPoolSpec:     dataName,
			PoolSpecs: []PoolSpec{
				{Name: metaName, PoolSpec: pool.ModelToSpec(store.MetadataConfig)},
				{Name: dataName, PoolSpec: pool.ModelToSpec(store.DataConfig)},
			},
			RGW: RGWSpec{
				Port:              store.RGW.Port,
				Replicas:          store.RGW.Replicas,
				SSLCertificateRef: store.RGW.CertificateRef,
			},
		},
	}
}

func (s *ObjectStore) makeDeployment(version string) *extensions.Deployment {
	deployment := &extensions.Deployment{}
	deployment.Name = s.instanceName()
	deployment.Namespace = s.Namespace

	podSpec := v1.PodSpec{
		Containers:    []v1.Container{s.rgwContainer(version)},
		RestartPolicy: v1.RestartPolicyAlways,
		Volumes: []v1.Volume{
			{Name: k8sutil.DataDirVolume, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
			k8sutil.ConfigOverrideVolume(),
		},
		HostNetwork: c.HostNetwork,
	}
	if c.HostNetwork {
		podSpec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}
	c.placement.ApplyToPodSpec(&podSpec)

	// Set the ssl cert if specified
	if s.Spec.RGW.SSLCertificateRef != "" {
		certVol := v1.Volume{Name: certVolumeName, VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{
			SecretName: s.Spec.RGW.SSLCertificateRef,
			Items:      []v1.KeyToPath{{Key: certKeyName, Path: certFilename}},
		}}}
		podSpec.Volumes = append(podSpec.Volumes, certVol)
	}

	s.Spec.RGW.Placement.ApplyToPodSpec(&podSpec)

	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:        s.instanceName(),
			Labels:      s.getLabels(),
			Annotations: map[string]string{},
		},
		Spec: podSpec,
	}

	deployment.Spec = extensions.DeploymentSpec{Template: podTemplateSpec, Replicas: &s.Spec.RGW.Replicas}

	return deployment
}

func (s *ObjectStore) rgwContainer(version string) v1.Container {

	container := v1.Container{
		Args: []string{
			"rgw",
			fmt.Sprintf("--config-dir=%s", k8sutil.DataDir),
			fmt.Sprintf("--rgw-name=%s", s.Name),
			fmt.Sprintf("--rgw-port=%d", s.Spec.RGW.Port),
			fmt.Sprintf("--rgw-host=%s", s.instanceName()),
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
			opmon.AdminSecretEnvVar(),
			k8sutil.ConfigOverrideEnvVar(),
		},
	}

	if s.Spec.RGW.SSLCertificateRef != "" {
		// Add a volume mount for the ssl certificate
		mount := v1.VolumeMount{Name: certVolumeName, MountPath: certMountPath, ReadOnly: true}
		container.VolumeMounts = append(container.VolumeMounts, mount)

		// Pass the flag for using the ssl cert
		path := path.Join(certMountPath, certFilename)
		container.Args = append(container.Args, fmt.Sprintf("--rgw-cert=%s", path))
	}

	return container
}

func (s *ObjectStore) startService(context *clusterd.Context) (string, error) {
	labels := s.getLabels()
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.instanceName(),
			Namespace: s.Namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       s.instanceName(),
					Port:       s.Spec.RGW.Port,
					TargetPort: intstr.FromInt(int(s.Spec.RGW.Port)),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector: labels,
		},
	}
	if c.HostNetwork {
		s.Spec.ClusterIP = v1.ClusterIPNone
	}

	svc, err := context.Clientset.CoreV1().Services(s.Namespace).Create(svc)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return "", fmt.Errorf("failed to create mon service. %+v", err)
		}
		logger.Infof("RGW service already running")
		return "", nil
	}

	logger.Infof("RGW service running at %s:%d", svc.Spec.ClusterIP, s.Spec.RGW.Port)
	return svc.Spec.ClusterIP, nil
}

func (s *ObjectStore) getLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: s.Namespace,
		"rook_object_store": s.Name,
	}
}
