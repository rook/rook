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

// Package S3 for the Edgefs manager.
package s3

import (
	"fmt"

	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	appName = "rook-edgefs-s3"

	/* Volumes definitions */
	serviceAccountName = "rook-edgefs-cluster"
	s3ImagePostfix     = "restapi"
	sslCertVolumeName  = "ssl-cert-volume"
	sslMountPath       = "/opt/nedge/etc/ssl/"
	dataVolumeName     = "edgefs-datadir"
	stateVolumeFolder  = ".state"
	etcVolumeFolder    = ".etc"
	defaultPort        = 9982
	defaultSecurePort  = 9443
	defaultServiceType = "ClusterIP"
)

// Start the S3 manager
func (c *S3Controller) CreateService(s edgefsv1.S3, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, false, ownerRefs)
}

func (c *S3Controller) UpdateService(s edgefsv1.S3, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, true, ownerRefs)
}

// Start the s3 instance
func (c *S3Controller) CreateOrUpdate(s edgefsv1.S3, update bool, ownerRefs []metav1.OwnerReference) error {
	logger.Debugf("starting update=%v service=%s", update, s.Name)

	// validate S3 service settings
	if err := validateService(c.context, s); err != nil {
		return fmt.Errorf("invalid S3 service %s arguments. %+v", s.Name, err)
	}

	//check http settings
	if s.Spec.Port == 0 {
		s.Spec.Port = defaultPort
	}

	if s.Spec.SecurePort == 0 {
		s.Spec.SecurePort = defaultSecurePort
	}

	if s.Spec.ServiceType == "" {
		s.Spec.ServiceType = defaultServiceType
	}

	imageArgs := "s3"
	rookImagePostfix := ""
	if s.Spec.S3Type == "" {
		rookImagePostfix = s3ImagePostfix
	} else if s.Spec.S3Type == "s3" || s.Spec.S3Type == "s3s" {
		// all rest APIs coming from edgefs-restapi image, that
		// includes mgmt, s3, s3s and swift
		rookImagePostfix = s3ImagePostfix
		imageArgs = s.Spec.S3Type
	} else if s.Spec.S3Type == "s3g" {
		// built-in s3 version, limited and experimental coming
		// as a part of core edgefs image
	} else {
		return fmt.Errorf("invalid S3 service type %s", s.Spec.S3Type)
	}

	// check if S3 service already exists
	exists, err := serviceExists(c.context, s)
	if err == nil && exists {
		if !update {
			logger.Infof("S3 service %s exists in namespace %s", s.Name, s.Namespace)
			return nil
		}
		logger.Infof("S3 service %s exists in namespace %s. checking for updates", s.Name, s.Namespace)
	}

	// start the deployment
	deployment := c.makeDeployment(s.Name, s.Namespace, edgefsv1.GetModifiedRookImagePath(c.rookImage, rookImagePostfix), imageArgs, s.Spec)
	if _, err := c.context.Clientset.AppsV1().Deployments(s.Namespace).Create(deployment); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s deployment. %+v", appName, err)
		}
		logger.Infof("%s deployment already exists", appName)
		if _, err := c.context.Clientset.AppsV1().Deployments(s.Namespace).Update(deployment); err != nil {
			return fmt.Errorf("failed to update %s deployment. %+v", appName, err)
		}
		logger.Infof("%s deployment updated", appName)
	} else {
		logger.Infof("%s deployment started", appName)
	}

	// create the s3 service
	service := c.makeS3Service(instanceName(s.Name), s.Name, s.Namespace, s.Spec)
	if _, err := c.context.Clientset.CoreV1().Services(s.Namespace).Create(service); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create s3 service. %+v", err)
		}
		logger.Infof("s3 service %s already exists", service)
	} else {
		logger.Infof("s3 service %s started", service)
	}

	return nil
}

// makeS3Service creates k8s service
func (c *S3Controller) makeS3Service(name, svcname, namespace string, s3Spec edgefsv1.S3Spec) *v1.Service {
	labels := getLabels(name, svcname, namespace)
	httpPort := v1.ServicePort{Name: "port", Port: int32(s3Spec.Port), Protocol: v1.ProtocolTCP}
	httpsPort := v1.ServicePort{Name: "secure-port", Port: int32(s3Spec.SecurePort), Protocol: v1.ProtocolTCP}

	if s3Spec.ExternalPort != 0 {
		httpPort.NodePort = int32(s3Spec.ExternalPort)
	}
	if s3Spec.SecureExternalPort != 0 {
		httpsPort.NodePort = int32(s3Spec.SecureExternalPort)
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     k8sutil.ParseServiceType(s3Spec.ServiceType),
			Ports: []v1.ServicePort{
				{Name: "grpc", Port: 49000, Protocol: v1.ProtocolTCP},
				httpPort,
				httpsPort,
			},
		},
	}

	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.ownerRef)
	return svc
}

func (c *S3Controller) makeDeployment(svcname, namespace, rookImage, imageArgs string, s3Spec edgefsv1.S3Spec) *apps.Deployment {
	name := instanceName(svcname)
	volumes := []v1.Volume{}

	if c.useHostLocalTime {
		volumes = append(volumes, edgefsv1.GetHostLocalTimeVolume())
	}

	// add ssl certificate volume if defined
	if len(s3Spec.SSLCertificateRef) > 0 {
		volumes = append(volumes, v1.Volume{
			Name: sslCertVolumeName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: s3Spec.SSLCertificateRef,
					Items: []v1.KeyToPath{
						{
							Key:  "sslkey",
							Path: "ssl.key",
						},
						{
							Key:  "sslcert",
							Path: "ssl.crt",
						},
					},
				},
			},
		})
	}

	if c.dataVolumeSize.Value() > 0 {
		// dataVolume case
		volumes = append(volumes, v1.Volume{
			Name: dataVolumeName,
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: dataVolumeName,
				},
			},
		})
	} else {
		// dataDir case
		volumes = append(volumes, v1.Volume{
			Name: dataVolumeName,
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: c.dataDirHostPath,
				},
			},
		})
	}

	podSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: getLabels(name, svcname, namespace),
		},
		Spec: v1.PodSpec{
			Containers:         []v1.Container{c.s3Container(svcname, name, rookImage, imageArgs, s3Spec)},
			RestartPolicy:      v1.RestartPolicyAlways,
			Volumes:            volumes,
			HostIPC:            true,
			HostNetwork:        c.NetworkSpec.IsHost(),
			NodeSelector:       map[string]string{namespace: "cluster"},
			ServiceAccountName: serviceAccountName,
		},
	}

	if c.NetworkSpec.IsHost() {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.NetworkSpec.IsMultus() {
		k8sutil.ApplyMultus(c.NetworkSpec, &podSpec.ObjectMeta)
	}

	// apply current S3 CRD options to pod's specification
	s3Spec.Placement.ApplyToPodSpec(&podSpec.Spec)
	s3Spec.Annotations.ApplyToObjectMeta(&podSpec.ObjectMeta)

	d := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: podSpec.Labels,
			},
			Template: podSpec,
			Replicas: &s3Spec.Instances,
		},
	}
	k8sutil.SetOwnerRef(&d.ObjectMeta, &c.ownerRef)
	s3Spec.Annotations.ApplyToObjectMeta(&d.ObjectMeta)
	return d
}

func (c *S3Controller) s3Container(svcname, name, containerImage, args string, s3Spec edgefsv1.S3Spec) v1.Container {
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		Capabilities: &v1.Capabilities{
			Add: []v1.Capability{"SYS_NICE", "SYS_RESOURCE", "IPC_LOCK"},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/etc.target",
			SubPath:   etcVolumeFolder,
		},
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/var/run",
			SubPath:   stateVolumeFolder,
		},
	}

	if c.useHostLocalTime {
		volumeMounts = append(volumeMounts, edgefsv1.GetHostLocalTimeVolumeMount())
	}

	if len(s3Spec.SSLCertificateRef) > 0 {
		volumeMounts = append(volumeMounts, v1.VolumeMount{Name: sslCertVolumeName, MountPath: sslMountPath})
	}

	cont := v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{args},
		Env: []v1.EnvVar{
			{
				Name:  "CCOW_LOG_LEVEL",
				Value: "5",
			},
			{
				Name:  "CCOW_SVCNAME",
				Value: svcname,
			},
			{
				Name:  "DEBUG",
				Value: "alert,error,info",
			},
			{
				Name: "HOST_HOSTNAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
			{
				Name: "K8S_NAMESPACE",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
			{
				Name:  "EFSS3_HTTP_PORT",
				Value: fmt.Sprint(s3Spec.Port),
			},
			{
				Name:  "EFSS3_HTTPS_PORT",
				Value: fmt.Sprint(s3Spec.SecurePort),
			},
		},
		SecurityContext: securityContext,
		Resources:       s3Spec.Resources,
		Ports: []v1.ContainerPort{
			{Name: "grpc", ContainerPort: 49000, Protocol: v1.ProtocolTCP},
			{Name: "port", ContainerPort: int32(s3Spec.Port), Protocol: v1.ProtocolTCP},
			{Name: "secure-port", ContainerPort: int32(s3Spec.SecurePort), Protocol: v1.ProtocolTCP},
		},
		VolumeMounts: volumeMounts,
	}

	cont.Env = append(cont.Env, edgefsv1.GetInitiatorEnvArr("s3",
		c.resourceProfile == "embedded" || s3Spec.ResourceProfile == "embedded",
		s3Spec.ChunkCacheSize, s3Spec.Resources)...)

	return cont
}

// Delete S3 service and possibly some artifacts.
func (c *S3Controller) DeleteService(s edgefsv1.S3) error {
	// check if service  exists
	exists, err := serviceExists(c.context, s)
	if err != nil {
		return fmt.Errorf("failed to detect if there is a S3 service to delete. %+v", err)
	}
	if !exists {
		logger.Infof("S3 service %s does not exist in namespace %s", s.Name, s.Namespace)
		return nil
	}

	logger.Infof("Deleting S3 service %s from namespace %s", s.Name, s.Namespace)

	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the s3 service
	err = c.context.Clientset.CoreV1().Services(s.Namespace).Delete(instanceName(s.Name), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete S3 service. %+v", err)
	}

	// Make a best effort to delete the S3 pods
	err = k8sutil.DeleteDeployment(c.context.Clientset, s.Namespace, instanceName(s.Name))
	if err != nil {
		logger.Warningf(err.Error())
	}

	logger.Infof("Completed deleting S3 service %s", s.Name)
	return nil
}

func getLabels(name, svcname, namespace string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: namespace,
		"edgefs_svcname":    svcname,
		"edgefs_svctype":    "s3",
	}
}

// Validate the S3 arguments
func validateService(context *clusterd.Context, s edgefsv1.S3) error {
	if s.Name == "" {
		return fmt.Errorf("missing name")
	}
	if s.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}

	return nil
}

func instanceName(svcname string) string {
	return fmt.Sprintf("%s-%s", appName, svcname)
}

// Check if the S3 service exists
func serviceExists(context *clusterd.Context, s edgefsv1.S3) (bool, error) {
	_, err := context.Clientset.AppsV1().Deployments(s.Namespace).Get(instanceName(s.Name), metav1.GetOptions{})
	if err == nil {
		// the deployment was found
		return true, nil
	}
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	// not found
	return false, nil
}
