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

// Package smb for the Edgefs manager.
package smb

import (
	"context"
	"fmt"
	"strings"

	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	appName = "rook-edgefs-smb"

	/* Volumes definitions */
	serviceAccountName = "rook-edgefs-cluster"
	dataVolumeName     = "edgefs-datadir"
	stateVolumeFolder  = ".state"
	etcVolumeFolder    = ".etc"
)

// Start the rgw manager
func (c *SMBController) CreateService(s edgefsv1.SMB, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, false, ownerRefs)
}

func (c *SMBController) UpdateService(s edgefsv1.SMB, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, true, ownerRefs)
}

// Start the smb instance
func (c *SMBController) CreateOrUpdate(s edgefsv1.SMB, update bool, ownerRefs []metav1.OwnerReference) error {
	ctx := context.TODO()
	logger.Infof("starting update=%v service=%s", update, s.Name)

	logger.Infof("SMB Base image is %s", c.rookImage)
	// validate SMB service settings
	if err := validateService(c.context, s); err != nil {
		return fmt.Errorf("invalid SMB service %s arguments. %+v", s.Name, err)
	}

	// check if SMB service already exists
	exists, err := serviceExists(ctx, c.context, s)
	if err == nil && exists {
		if !update {
			logger.Infof("SMB service %s exists in namespace %s", s.Name, s.Namespace)
			return nil
		}
		logger.Infof("SMB service %s exists in namespace %s. checking for updates", s.Name, s.Namespace)
	}

	// start the deployment
	deployment := c.makeDeployment(s.Name, s.Namespace, c.rookImage, s.Spec)
	if _, err := c.context.Clientset.AppsV1().Deployments(s.Namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create %s deployment. %+v", appName, err)
		}

		logger.Infof("%s deployment already exists", appName)
		if _, err := c.context.Clientset.AppsV1().Deployments(s.Namespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update %s deployment. %+v", appName, err)
		}
		logger.Infof("%s deployment updated", appName)
	} else {
		logger.Infof("%s deployment started", appName)
	}

	// create the smb service
	service := c.makeSMBService(instanceName(s.Name), s.Name, s.Namespace)
	if _, err := c.context.Clientset.CoreV1().Services(s.Namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create smb service. %+v", err)
		}
		logger.Infof("smb service %s already exists", service)
	} else {
		logger.Infof("smb service %s started", service)
	}

	return nil
}

func (c *SMBController) makeSMBService(name, svcname, namespace string) *v1.Service {
	labels := getLabels(name, svcname, namespace)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{Name: "grpc", Port: 49000, Protocol: v1.ProtocolTCP},
				{Name: "netbios-ns", Port: 137, Protocol: v1.ProtocolTCP},
				{Name: "netbios-ns-udp", Port: 137, Protocol: v1.ProtocolUDP},
				{Name: "netbios-dgm", Port: 138, Protocol: v1.ProtocolTCP},
				{Name: "netbios-dgm-udp", Port: 138, Protocol: v1.ProtocolUDP},
				{Name: "netbios-ssn", Port: 139, Protocol: v1.ProtocolTCP},
				{Name: "microsoft-ds", Port: 445, Protocol: v1.ProtocolTCP},
			},
		},
	}

	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.ownerRef)
	return svc
}

func (c *SMBController) makeDeployment(svcname, namespace, rookImage string, smbSpec edgefsv1.SMBSpec) *apps.Deployment {
	name := instanceName(svcname)
	volumes := []v1.Volume{}

	if c.useHostLocalTime {
		volumes = append(volumes, edgefsv1.GetHostLocalTimeVolume())
		volumes = append(volumes, edgefsv1.GetHostTimeZoneVolume())
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
			Containers:         []v1.Container{c.smbContainer(svcname, name, rookImage, smbSpec)},
			RestartPolicy:      v1.RestartPolicyAlways,
			Volumes:            volumes,
			HostIPC:            true,
			HostNetwork:        c.NetworkSpec.IsHost(),
			NodeSelector:       map[string]string{namespace: "cluster"},
			ServiceAccountName: serviceAccountName,
		},
	}

	if smbSpec.Ads != (edgefsv1.AdsSpec{}) {
		podSpec.Spec.DNSPolicy = v1.DNSNone
		podSpec.Spec.DNSConfig = &v1.PodDNSConfig{
			Nameservers: strings.Split(smbSpec.Ads.Nameservers, ","),
			Searches:    []string{strings.ToLower(smbSpec.Ads.DomainName)},
		}
	}

	if c.NetworkSpec.IsHost() {
		podSpec.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	} else if c.NetworkSpec.IsMultus() {
		if err := k8sutil.ApplyMultus(c.NetworkSpec, &podSpec.ObjectMeta); err != nil {
			logger.Errorf("failed to apply multus spec to podspec metadata for smb. %v", err)
		}
	}
	smbSpec.Annotations.ApplyToObjectMeta(&podSpec.ObjectMeta)

	// apply current SMB CRD options to pod's specification
	smbSpec.Placement.ApplyToPodSpec(&podSpec.Spec, true)

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
			Replicas: &smbSpec.Instances,
		},
	}
	k8sutil.SetOwnerRef(&d.ObjectMeta, &c.ownerRef)
	smbSpec.Annotations.ApplyToObjectMeta(&d.ObjectMeta)
	return d
}

func (c *SMBController) smbContainer(svcname, name, containerImage string, smbSpec edgefsv1.SMBSpec) v1.Container {
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
		volumeMounts = append(volumeMounts, edgefsv1.GetHostTimeZoneVolumeMount())
	}

	cont := v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{"smb"},
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
				Name: "K8S_NAMESPACE",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
		},
		SecurityContext: securityContext,
		Resources:       smbSpec.Resources,
		Ports: []v1.ContainerPort{
			{Name: "grpc", ContainerPort: 49000, Protocol: v1.ProtocolTCP},
			{Name: "netbios-ns", ContainerPort: 137, Protocol: v1.ProtocolTCP},
			{Name: "netbios-ns-udp", ContainerPort: 137, Protocol: v1.ProtocolUDP},
			{Name: "netbios-dgm", ContainerPort: 138, Protocol: v1.ProtocolTCP},
			{Name: "netbios-dgm-udp", ContainerPort: 138, Protocol: v1.ProtocolUDP},
			{Name: "netbios-ssn", ContainerPort: 139, Protocol: v1.ProtocolTCP},
			{Name: "microsoft-ds", ContainerPort: 445, Protocol: v1.ProtocolTCP},
		},
		VolumeMounts: volumeMounts,
	}

	if smbSpec.Ads != (edgefsv1.AdsSpec{}) {
		adsDN := smbSpec.Ads.DomainName
		cont.Env = append(cont.Env, v1.EnvVar{
			Name:  "EFSSMB_DOMAIN_NAME",
			Value: adsDN,
		})
		adsShortName := strings.Split(adsDN, ".")[0]
		cont.Env = append(cont.Env, v1.EnvVar{
			Name:  "EFSSMB_WORKGROUP",
			Value: adsShortName,
		})
		cont.Env = append(cont.Env, v1.EnvVar{
			Name:  "EFSSMB_DC1",
			Value: smbSpec.Ads.DcName,
		})
		cont.Env = append(cont.Env, v1.EnvVar{
			Name:  "EFSSMB_NETBIOS_NAME",
			Value: smbSpec.Ads.ServerName,
		})
		cont.Env = append(cont.Env, v1.EnvVar{
			Name: "EFSSMB_AD_USERNAME",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{Name: smbSpec.Ads.UserSecret},
					Key:                  "username",
				},
			},
		})
		cont.Env = append(cont.Env, v1.EnvVar{
			Name: "EFSSMB_AD_PASSWORD",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{Name: smbSpec.Ads.UserSecret},
					Key:                  "password",
				},
			},
		})
	}

	if smbSpec.RelaxedDirUpdates {
		cont.Env = append(cont.Env, v1.EnvVar{
			Name:  "EFSSMB_RELAXED_DIR_UPDATES",
			Value: "1",
		})
	}

	cont.Env = append(cont.Env, edgefsv1.GetInitiatorEnvArr("smb",
		c.resourceProfile == "embedded" || smbSpec.ResourceProfile == "embedded",
		smbSpec.ChunkCacheSize, smbSpec.Resources)...)

	return cont
}

// Delete SMB service and possibly some artifacts.
func (c *SMBController) DeleteService(s edgefsv1.SMB) error {
	ctx := context.TODO()
	// check if service  exists
	exists, err := serviceExists(ctx, c.context, s)
	if err != nil {
		return fmt.Errorf("failed to detect if there is a SMB service to delete. %+v", err)
	}
	if !exists {
		logger.Infof("SMB service %s does not exist in namespace %s", s.Name, s.Namespace)
		return nil
	}

	logger.Infof("Deleting SMB service %s from namespace %s", s.Name, s.Namespace)

	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the smb service
	err = c.context.Clientset.CoreV1().Services(s.Namespace).Delete(ctx, instanceName(s.Name), *options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete SMB service. %+v", err)
	}

	// Make a best effort to delete the SMB pods
	err = k8sutil.DeleteDeployment(c.context.Clientset, s.Namespace, instanceName(s.Name))
	if err != nil {
		logger.Warningf(err.Error())
	}

	logger.Infof("Completed deleting SMB service %s", s.Name)
	return nil
}

func getLabels(name, svcname, namespace string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: namespace,
		"edgefs_svcname":    svcname,
		"edgefs_svctype":    "smb",
	}
}

// Validate the SMB arguments
func validateService(context *clusterd.Context, s edgefsv1.SMB) error {
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

// Check if the SMB service exists
func serviceExists(ctx context.Context, context *clusterd.Context, s edgefsv1.SMB) (bool, error) {
	_, err := context.Clientset.AppsV1().Deployments(s.Namespace).Get(ctx, instanceName(s.Name), metav1.GetOptions{})
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
