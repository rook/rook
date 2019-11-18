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

// Package ISGW for the Edgefs manager.
package isgw

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	appName = "rook-edgefs-isgw"

	/* ISGW definitions */
	serviceAccountName      = "rook-edgefs-cluster"
	defaultReplicationType  = "initial+continuous"
	defaultDynamicFetchPort = 49678
	defaultLocalIPAddr      = "0.0.0.0"
	defaultLocalIPv6Addr    = "::"
	defaultLocalPort        = 14000
	dataVolumeName          = "edgefs-datadir"
	stateVolumeFolder       = ".state"
	etcVolumeFolder         = ".etc"
)

// Start the ISGW manager
func (c *ISGWController) CreateService(s edgefsv1.ISGW, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, false, ownerRefs)
}

func (c *ISGWController) UpdateService(s edgefsv1.ISGW, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, true, ownerRefs)
}

// Start the isgw instance
func (c *ISGWController) CreateOrUpdate(s edgefsv1.ISGW, update bool, ownerRefs []metav1.OwnerReference) error {
	logger.Infof("starting update=%v service=%s", update, s.Name)

	logger.Infof("ISGW Base image is %s", c.rookImage)
	// validate ISGW service settings
	if err := validateService(c.context, s); err != nil {
		return fmt.Errorf("invalid ISGW service %s arguments. %+v", s.Name, err)
	}

	if s.Spec.ReplicationType == "" {
		s.Spec.ReplicationType = defaultReplicationType
	}

	if s.Spec.DynamicFetchAddr == "" {
		s.Spec.DynamicFetchAddr = "-"
	}

	if s.Spec.LocalAddr == "" {
		s.Spec.LocalAddr = defaultLocalIPAddr + ":" + strconv.Itoa(defaultLocalPort)
	}

	configJSON := getISGWConfigJSON(s.Spec)
	if len(configJSON) == 0 && len(s.Spec.RemoteURL) == 0 {
		return fmt.Errorf("If no additional configuration specified the RemoteURL should be presented")
	}

	// check if ISGW service already exists
	exists, err := serviceExists(c.context, s)
	if err == nil && exists {
		if !update {
			logger.Infof("ISGW service %s exists in namespace %s", s.Name, s.Namespace)
			return nil
		}
		logger.Infof("ISGW service %s exists in namespace %s. checking for updates", s.Name, s.Namespace)
	}

	// start the deployment
	deployment := c.makeDeployment(s.Name, s.Namespace, c.rookImage, s.Spec)
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

	// create the isgw service
	service := c.makeISGWService(instanceName(s.Name), s.Name, s.Namespace, s.Spec)
	if _, err := c.context.Clientset.CoreV1().Services(s.Namespace).Create(service); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create ISGW service. %+v", err)
		}
		logger.Infof("ISGW service %s already exists", service)
	} else {
		logger.Infof("ISGW service %s started", service)
	}

	return nil
}

func getDirection(isgwSpec edgefsv1.ISGWSpec) int {
	direction := 0
	if isgwSpec.Direction == "send" {
		direction = 1
	} else if isgwSpec.Direction == "receive" {
		direction = 2
	} else {
		direction = 3
	}
	return direction
}

func getISGWConfigJSON(isgwSpec edgefsv1.ISGWSpec) string {
	result := ""
	if len(isgwSpec.Config.Server) > 0 || len(isgwSpec.Config.Clients) > 0 {
		bytes, _ := json.Marshal(isgwSpec.Config)
		if string(bytes) != "{}" {
			result = string(bytes)
		}
	}
	return result
}

func (c *ISGWController) makeISGWService(name, svcname, namespace string, isgwSpec edgefsv1.ISGWSpec) *v1.Service {
	direction := getDirection(isgwSpec)
	labels := getLabels(name, svcname, namespace)
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Selector: labels,
			Type:     k8sutil.ParseServiceType(isgwSpec.ServiceType),
			Ports: []v1.ServicePort{
				{Name: "grpc", Port: 49000, Protocol: v1.ProtocolTCP},
			},
		},
	}

	if direction == 2 || direction == 3 {
		laddr, port, err := net.SplitHostPort(isgwSpec.LocalAddr)
		if err != nil {
			logger.Errorf("wrong localAddr format")
			return svc
		}
		lport, _ := strconv.Atoi(port)
		lportServicePort := v1.ServicePort{Name: "lport", Port: int32(lport), Protocol: v1.ProtocolTCP}
		if isgwSpec.ExternalPort != 0 {
			lportServicePort.NodePort = int32(isgwSpec.ExternalPort)
		}
		svc.Spec.Ports = append(svc.Spec.Ports, lportServicePort)

		if laddr != defaultLocalIPAddr && laddr != defaultLocalIPv6Addr {
			logger.Infof("ISGW service %s assigned with externalIP=%s", svcname, laddr)
			svc.Spec.ExternalIPs = []string{laddr}
		}
	}
	if (direction == 1 || direction == 3) && isgwSpec.DynamicFetchAddr != "-" {
		_, port, err := net.SplitHostPort(isgwSpec.DynamicFetchAddr)
		if err != nil {
			logger.Errorf("wrong dynamicFetchAddr format")
			return svc
		}
		lport, _ := strconv.Atoi(port)

		svc.Spec.Ports = append(svc.Spec.Ports, v1.ServicePort{Name: "dfport", Port: int32(lport), Protocol: v1.ProtocolTCP})
	}

	k8sutil.SetOwnerRef(&svc.ObjectMeta, &c.ownerRef)
	return svc
}

func (c *ISGWController) makeDeployment(svcname, namespace, rookImage string, isgwSpec edgefsv1.ISGWSpec) *apps.Deployment {
	name := instanceName(svcname)
	volumes := []v1.Volume{}

	if c.useHostLocalTime {
		volumes = append(volumes, edgefsv1.GetHostLocalTimeVolume())
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
			Containers:         []v1.Container{c.isgwContainer(svcname, name, edgefsv1.GetModifiedRookImagePath(rookImage, "isgw"), isgwSpec)},
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

	// apply current ISGW CRD options to pod's specification
	isgwSpec.Placement.ApplyToPodSpec(&podSpec.Spec)

	instancesCount := int32(1)
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
			Replicas: &instancesCount,
		},
	}
	k8sutil.SetOwnerRef(&d.ObjectMeta, &c.ownerRef)
	return d
}

func (c *ISGWController) isgwContainer(svcname, name, containerImage string, isgwSpec edgefsv1.ISGWSpec) v1.Container {
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		Capabilities: &v1.Capabilities{
			Add: []v1.Capability{"SYS_NICE", "SYS_RESOURCE", "IPC_LOCK"},
		},
	}

	direction := getDirection(isgwSpec)

	replication := 3
	if isgwSpec.ReplicationType == "initial" {
		replication = 1
	} else if isgwSpec.ReplicationType == "continuous" {
		replication = 2
	} else {
		replication = 3
	}

	mdonly := 0
	if isgwSpec.MetadataOnly == "versions" {
		mdonly = 2
	} else if isgwSpec.MetadataOnly == "all" {
		mdonly = 1
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

	configJSON := getISGWConfigJSON(isgwSpec)

	cont := v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{"isgw"},
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
			{
				Name:  "EFSISGW_DIRECTION",
				Value: strconv.Itoa(direction),
			},
			{
				Name:  "EFSISGW_REMOTE_URL",
				Value: isgwSpec.RemoteURL,
			},
			{
				Name:  "EFSISGW_LOCAL_ADDR",
				Value: isgwSpec.LocalAddr,
			},
			{
				Name:  "EFSISGW_DYNAMIC_FETCH_ADDR",
				Value: isgwSpec.DynamicFetchAddr,
			},
			{
				Name:  "EFSISGW_REPLICATION_TYPE",
				Value: strconv.Itoa(replication),
			},
			{
				Name:  "EFSISGW_USE_ENCRYPTED_TUNNEL",
				Value: (map[bool]string{true: "1", false: "0"})[isgwSpec.UseEncryptedTunnel],
			},
			{
				Name:  "EFSISGW_METADATA_ONLY",
				Value: strconv.Itoa(mdonly),
			},
			{
				Name:  "EFSISGW_CONFIGURATION",
				Value: configJSON,
			},
		},
		SecurityContext: securityContext,
		Resources:       isgwSpec.Resources,
		Ports: []v1.ContainerPort{
			{Name: "grpc", ContainerPort: 49000, Protocol: v1.ProtocolTCP},
		},
		VolumeMounts: volumeMounts,
	}

	if direction == 2 || direction == 3 {
		_, port, err := net.SplitHostPort(isgwSpec.LocalAddr)
		if err != nil {
			logger.Errorf("wrong localAddr format")
			return cont
		}
		lport, _ := strconv.Atoi(port)
		cont.Ports = append(cont.Ports, v1.ContainerPort{Name: "lport", ContainerPort: int32(lport), Protocol: v1.ProtocolTCP})
	}
	if (direction == 1 || direction == 3) && isgwSpec.DynamicFetchAddr != "-" {
		_, port, err := net.SplitHostPort(isgwSpec.DynamicFetchAddr)
		if err != nil {
			logger.Errorf("wrong dynamicFetchAddr format")
			return cont
		}
		lport, _ := strconv.Atoi(port)
		cont.Ports = append(cont.Ports, v1.ContainerPort{Name: "dfport", ContainerPort: int32(lport), Protocol: v1.ProtocolTCP})
	}

	cont.Env = append(cont.Env, edgefsv1.GetInitiatorEnvArr("isgw",
		c.resourceProfile == "embedded" || isgwSpec.ResourceProfile == "embedded",
		isgwSpec.ChunkCacheSize, isgwSpec.Resources)...)

	return cont
}

// Delete ISGW service and possibly some artifacts.
func (c *ISGWController) DeleteService(s edgefsv1.ISGW) error {
	// check if service  exists
	exists, err := serviceExists(c.context, s)
	if err != nil {
		return fmt.Errorf("failed to detect if there is a ISGW service to delete. %+v", err)
	}
	if !exists {
		logger.Infof("ISGW service %s does not exist in namespace %s", s.Name, s.Namespace)
		return nil
	}

	logger.Infof("Deleting ISGW service %s from namespace %s", s.Name, s.Namespace)

	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the isgw service
	err = c.context.Clientset.CoreV1().Services(s.Namespace).Delete(instanceName(s.Name), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete ISGW service. %+v", err)
	}

	// Make a best effort to delete the ISGW pods
	err = k8sutil.DeleteDeployment(c.context.Clientset, s.Namespace, instanceName(s.Name))
	if err != nil {
		logger.Warningf(err.Error())
	}

	logger.Infof("Completed deleting ISGW service %s", s.Name)
	return nil
}

func getLabels(name, svcname, namespace string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: namespace,
		"edgefs_svcname":    svcname,
		"edgefs_svctype":    "isgw",
	}
}

// Validate the ISGW arguments
func validateService(context *clusterd.Context, s edgefsv1.ISGW) error {
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

// Check if the ISGW service exists
func serviceExists(context *clusterd.Context, s edgefsv1.ISGW) (bool, error) {
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
