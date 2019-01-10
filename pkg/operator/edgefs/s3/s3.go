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

// Package S3 for the Edgefs manager.
package s3

import (
	"fmt"

	edgefsv1alpha1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	appName = "rook-edgefs-s3"

	/* Volumes definitions */
	sslCertVolumeName = "ssl-cert-volume"
	sslMountPath      = "/opt/nedge/etc/ssl/"
	dataVolumeName    = "edgefs-datadir"
	stateVolumeFolder = ".state"
	etcVolumeFolder   = ".etc"
	defaultPort       = 9982
	defaultSecurePort = 8443
)

// Start the S3 manager
func (c *S3Controller) CreateService(s edgefsv1alpha1.S3, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, false, ownerRefs)
}

func (c *S3Controller) UpdateService(s edgefsv1alpha1.S3, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, true, ownerRefs)
}

// Start the s3 instance
func (c *S3Controller) CreateOrUpdate(s edgefsv1alpha1.S3, update bool, ownerRefs []metav1.OwnerReference) error {
	logger.Infof("starting update=%v service=%s", update, s.Name)

	logger.Infof("S3 Base image is %s", c.rookImage)
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
	deployment := c.makeDeployment(s.Name, s.Namespace, c.rookImage, s.Spec)
	if _, err := c.context.Clientset.ExtensionsV1beta1().Deployments(s.Namespace).Create(deployment); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s deployment. %+v", appName, err)
		}
		logger.Infof("%s deployment already exists", appName)
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

func (c *S3Controller) makeS3Service(name, svcname, namespace string, s3Spec edgefsv1alpha1.S3Spec) *v1.Service {
	labels := getLabels(name, svcname, namespace)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeNodePort,
			Ports: []v1.ServicePort{
				{Name: "grpc", Port: 49000, Protocol: v1.ProtocolTCP},
				{Name: "port", Port: int32(s3Spec.Port), Protocol: v1.ProtocolTCP},
				{Name: "secure-port", Port: int32(s3Spec.SecurePort), Protocol: v1.ProtocolTCP},
			},
		},
	}

	k8sutil.SetOwnerRef(c.context.Clientset, namespace, &svc.ObjectMeta, &c.ownerRef)
	return svc
}

func (c *S3Controller) makeDeployment(svcname, namespace, rookImage string, s3Spec edgefsv1alpha1.S3Spec) *extensions.Deployment {

	name := instanceName(svcname)
	volumes := []v1.Volume{}

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
			Containers:    []v1.Container{c.s3Container(svcname, name, rookImage, s3Spec)},
			RestartPolicy: v1.RestartPolicyAlways,
			Volumes:       volumes,
			HostIPC:       true,
			HostNetwork:   c.hostNetwork,
			NodeSelector:  map[string]string{namespace: "cluster"},
		},
	}
	if c.hostNetwork {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	}

	// apply current S3 CRD options to pod's specification
	s3Spec.Placement.ApplyToPodSpec(&podSpec.Spec)

	d := &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: extensions.DeploymentSpec{Template: podSpec, Replicas: &s3Spec.Instances},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, namespace, &d.ObjectMeta, &c.ownerRef)
	return d
}

func (c *S3Controller) s3Container(svcname, name, containerImage string, s3Spec edgefsv1alpha1.S3Spec) v1.Container {

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

	if len(s3Spec.SSLCertificateRef) > 0 {
		volumeMounts = append(volumeMounts, v1.VolumeMount{Name: sslCertVolumeName, MountPath: sslMountPath})
	}

	return v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{"s3"},
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
				Name: "HOST_HOSTNAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
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
}

// Delete S3 service and possibly some artifacts.
func (c *S3Controller) DeleteService(s edgefsv1alpha1.S3) error {
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
func validateService(context *clusterd.Context, s edgefsv1alpha1.S3) error {
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
func serviceExists(context *clusterd.Context, s edgefsv1alpha1.S3) (bool, error) {
	_, err := context.Clientset.ExtensionsV1beta1().Deployments(s.Namespace).Get(instanceName(s.Name), metav1.GetOptions{})
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
