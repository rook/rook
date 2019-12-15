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

// Package SWIFT for the Edgefs manager.
package swift

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
	appName = "rook-edgefs-swift"

	/* Volumes definitions */
	serviceAccountName = "rook-edgefs-cluster"
	swiftImagePostfix  = "restapi"
	sslCertVolumeName  = "ssl-cert-volume"
	sslMountPath       = "/opt/nedge/etc/ssl/"
	dataVolumeName     = "edgefs-datadir"
	stateVolumeFolder  = ".state"
	etcVolumeFolder    = ".etc"
	defaultPort        = 9981
	defaultSecurePort  = 443
	defaultServiceType = "ClusterIP"
)

// Start the SWIFT manager
func (c *SWIFTController) CreateService(s edgefsv1.SWIFT, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, false, ownerRefs)
}

func (c *SWIFTController) UpdateService(s edgefsv1.SWIFT, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, true, ownerRefs)
}

// Start the swift instance
func (c *SWIFTController) CreateOrUpdate(s edgefsv1.SWIFT, update bool, ownerRefs []metav1.OwnerReference) error {
	logger.Debugf("starting update=%v service=%s", update, s.Name)

	// validate SWIFT service settings
	if err := validateService(c.context, s); err != nil {
		return fmt.Errorf("invalid SWIFT service %s arguments. %+v", s.Name, err)
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

	// all rest APIs coming from edgefs-restapi image, that
	// includes mgmt, s3, s3s and swift
	imageArgs := "swift"
	rookImage := edgefsv1.GetModifiedRookImagePath(c.rookImage, swiftImagePostfix)

	// check if SWIFT service already exists
	exists, err := serviceExists(c.context, s)
	if err == nil && exists {
		if !update {
			logger.Infof("SWIFT service %s exists in namespace %s", s.Name, s.Namespace)
			return nil
		}
		logger.Infof("SWIFT service %s exists in namespace %s. checking for updates", s.Name, s.Namespace)
	}

	// start the deployment
	deployment := c.makeDeployment(s.Name, s.Namespace, rookImage, imageArgs, s.Spec)
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

	// create the swift service
	service := c.makeSWIFTService(instanceName(s.Name), s.Name, s.Namespace, s.Spec)
	if _, err := c.context.Clientset.CoreV1().Services(s.Namespace).Create(service); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create swift service. %+v", err)
		}
		logger.Infof("swift service %s already exists", service)
	} else {
		logger.Infof("swift service %s started", service)
	}

	return nil
}

func (c *SWIFTController) makeSWIFTService(name, svcname, namespace string, swiftSpec edgefsv1.SWIFTSpec) *v1.Service {
	labels := getLabels(name, svcname, namespace)
	httpPort := v1.ServicePort{Name: "port", Port: int32(swiftSpec.Port), Protocol: v1.ProtocolTCP}
	httpsPort := v1.ServicePort{Name: "secure-port", Port: int32(swiftSpec.SecurePort), Protocol: v1.ProtocolTCP}

	if swiftSpec.ExternalPort != 0 {
		httpPort.NodePort = int32(swiftSpec.ExternalPort)
	}
	if swiftSpec.SecureExternalPort != 0 {
		httpsPort.NodePort = int32(swiftSpec.SecureExternalPort)
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     k8sutil.ParseServiceType(swiftSpec.ServiceType),
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

func (c *SWIFTController) makeDeployment(svcname, namespace, rookImage, imageArgs string, swiftSpec edgefsv1.SWIFTSpec) *apps.Deployment {
	name := instanceName(svcname)
	volumes := []v1.Volume{}

	if c.useHostLocalTime {
		volumes = append(volumes, edgefsv1.GetHostLocalTimeVolume())
	}

	// add ssl certificate volume if defined
	if len(swiftSpec.SSLCertificateRef) > 0 {
		volumes = append(volumes, v1.Volume{
			Name: sslCertVolumeName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: swiftSpec.SSLCertificateRef,
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
			Containers:         []v1.Container{c.swiftContainer(svcname, name, rookImage, imageArgs, swiftSpec)},
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
	}

	// apply current SWIFT CRD options to pod's specification
	swiftSpec.Placement.ApplyToPodSpec(&podSpec.Spec)

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
			Replicas: &swiftSpec.Instances,
		},
	}
	k8sutil.SetOwnerRef(&d.ObjectMeta, &c.ownerRef)
	return d
}

func (c *SWIFTController) swiftContainer(svcname, name, containerImage, args string, swiftSpec edgefsv1.SWIFTSpec) v1.Container {
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

	if len(swiftSpec.SSLCertificateRef) > 0 {
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
				Name:  "EFSSWIFT_HTTP_PORT",
				Value: fmt.Sprint(swiftSpec.Port),
			},
			{
				Name:  "EFSSWIFT_HTTPS_PORT",
				Value: fmt.Sprint(swiftSpec.SecurePort),
			},
		},
		SecurityContext: securityContext,
		Resources:       swiftSpec.Resources,
		Ports: []v1.ContainerPort{
			{Name: "grpc", ContainerPort: 49000, Protocol: v1.ProtocolTCP},
			{Name: "port", ContainerPort: int32(swiftSpec.Port), Protocol: v1.ProtocolTCP},
			{Name: "secure-port", ContainerPort: int32(swiftSpec.SecurePort), Protocol: v1.ProtocolTCP},
		},
		VolumeMounts: volumeMounts,
	}

	cont.Env = append(cont.Env, edgefsv1.GetInitiatorEnvArr("swift",
		c.resourceProfile == "embedded" || swiftSpec.ResourceProfile == "embedded",
		swiftSpec.ChunkCacheSize, swiftSpec.Resources)...)

	return cont
}

// Delete SWIFT service and possibly some artifacts.
func (c *SWIFTController) DeleteService(s edgefsv1.SWIFT) error {
	// check if service  exists
	exists, err := serviceExists(c.context, s)
	if err != nil {
		return fmt.Errorf("failed to detect if there is a SWIFT service to delete. %+v", err)
	}
	if !exists {
		logger.Infof("SWIFT service %s does not exist in namespace %s", s.Name, s.Namespace)
		return nil
	}

	logger.Infof("Deleting SWIFT service %s from namespace %s", s.Name, s.Namespace)

	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the swift service
	err = c.context.Clientset.CoreV1().Services(s.Namespace).Delete(instanceName(s.Name), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete SWIFT service. %+v", err)
	}

	// Make a best effort to delete the SWIFT pods
	err = k8sutil.DeleteDeployment(c.context.Clientset, s.Namespace, instanceName(s.Name))
	if err != nil {
		logger.Warningf(err.Error())
	}

	logger.Infof("Completed deleting SWIFT service %s", s.Name)
	return nil
}

func getLabels(name, svcname, namespace string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: namespace,
		"edgefs_svcname":    svcname,
		"edgefs_svctype":    "swift",
	}
}

// Validate the SWIFT arguments
func validateService(context *clusterd.Context, s edgefsv1.SWIFT) error {
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

// Check if the SWIFT service exists
func serviceExists(context *clusterd.Context, s edgefsv1.SWIFT) (bool, error) {
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
