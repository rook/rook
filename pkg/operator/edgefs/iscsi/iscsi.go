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

// Package ISCSI for the Edgefs manager.
package iscsi

import (
	"encoding/json"
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
	appName = "rook-edgefs-iscsi"

	/* Volumes definitions */
	defaultTargetName   = "iqn.2018-11.edgefs.io"
	defaultTargetParams = "{}"
	dataVolumeName      = "edgefs-datadir"
	stateVolumeFolder   = ".state"
	etcVolumeFolder     = ".etc"
	defaultPort         = 3260
)

// Start the ISCSI manager
func (c *ISCSIController) CreateService(s edgefsv1alpha1.ISCSI, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, false, ownerRefs)
}

func (c *ISCSIController) UpdateService(s edgefsv1alpha1.ISCSI, ownerRefs []metav1.OwnerReference) error {
	return c.CreateOrUpdate(s, true, ownerRefs)
}

// Start the iscsi instance
func (c *ISCSIController) CreateOrUpdate(s edgefsv1alpha1.ISCSI, update bool, ownerRefs []metav1.OwnerReference) error {
	logger.Infof("starting update=%v service=%s", update, s.Name)

	logger.Infof("ISCSI Base image is %s", c.rookImage)
	// validate ISCSI service settings
	if err := validateService(c.context, s); err != nil {
		return fmt.Errorf("invalid ISCSI service %s arguments. %+v", s.Name, err)
	}

	if len(s.Spec.TargetName) == 0 {
		s.Spec.TargetName = defaultTargetName
	}

	// check if ISCSI service already exists
	exists, err := serviceExists(c.context, s)
	if err == nil && exists {
		if !update {
			logger.Infof("ISCSI service %s exists in namespace %s", s.Name, s.Namespace)
			return nil
		}
		logger.Infof("ISCSI service %s exists in namespace %s. checking for updates", s.Name, s.Namespace)
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

	// create the iscsi service
	service := c.makeISCSIService(instanceName(s.Name), s.Name, s.Namespace, s.Spec)
	if _, err := c.context.Clientset.CoreV1().Services(s.Namespace).Create(service); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create ISCSI service. %+v", err)
		}
		logger.Infof("ISCSI service %s already exists", service)
	} else {
		logger.Infof("ISCSI service %s started", service)
	}

	return nil
}

func (c *ISCSIController) makeISCSIService(name, svcname, namespace string, iscsiSpec edgefsv1alpha1.ISCSISpec) *v1.Service {
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
				{Name: "port", Port: defaultPort, Protocol: v1.ProtocolTCP},
			},
		},
	}

	k8sutil.SetOwnerRef(c.context.Clientset, namespace, &svc.ObjectMeta, &c.ownerRef)
	return svc
}

func (c *ISCSIController) makeDeployment(svcname, namespace, rookImage string, iscsiSpec edgefsv1alpha1.ISCSISpec) *extensions.Deployment {

	name := instanceName(svcname)
	volumes := []v1.Volume{}

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
			Containers:    []v1.Container{c.iscsiContainer(svcname, name, rookImage, iscsiSpec)},
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

	// apply current ISCSI CRD options to pod's specification
	iscsiSpec.Placement.ApplyToPodSpec(&podSpec.Spec)

	instancesCount := int32(1)
	d := &extensions.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: extensions.DeploymentSpec{Template: podSpec, Replicas: &instancesCount},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, namespace, &d.ObjectMeta, &c.ownerRef)
	return d
}

func (c *ISCSIController) iscsiContainer(svcname, name, containerImage string, iscsiSpec edgefsv1alpha1.ISCSISpec) v1.Container {

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

	return v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{"iscsi"},
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
				Name: "KUBERNETES_NAMESPACE",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
			{
				Name:  "EFSISCSI_TARGET_NAME",
				Value: iscsiSpec.TargetName,
			},
			{
				Name:  "EFSISCSI_TARGET_PARAMS",
				Value: getTargetParamsJSON(iscsiSpec.TargetParams),
			},
		},
		SecurityContext: securityContext,
		Resources:       iscsiSpec.Resources,
		Ports: []v1.ContainerPort{
			{Name: "grpc", ContainerPort: 49000, Protocol: v1.ProtocolTCP},
			{Name: "port", ContainerPort: defaultPort, Protocol: v1.ProtocolTCP},
		},
		VolumeMounts: volumeMounts,
	}
}

// Delete ISCSI service and possibly some artifacts.
func (c *ISCSIController) DeleteService(s edgefsv1alpha1.ISCSI) error {
	// check if service  exists
	exists, err := serviceExists(c.context, s)
	if err != nil {
		return fmt.Errorf("failed to detect if there is a ISCSI service to delete. %+v", err)
	}
	if !exists {
		logger.Infof("ISCSI service %s does not exist in namespace %s", s.Name, s.Namespace)
		return nil
	}

	logger.Infof("Deleting ISCSI service %s from namespace %s", s.Name, s.Namespace)

	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the iscsi service
	err = c.context.Clientset.CoreV1().Services(s.Namespace).Delete(instanceName(s.Name), options)
	if err != nil && !errors.IsNotFound(err) {
		logger.Warningf("failed to delete ISCSI service. %+v", err)
	}

	// Make a best effort to delete the ISCSI pods
	err = k8sutil.DeleteDeployment(c.context.Clientset, s.Namespace, instanceName(s.Name))
	if err != nil {
		logger.Warningf(err.Error())
	}

	logger.Infof("Completed deleting ISCSI service %s", s.Name)
	return nil
}

func getLabels(name, svcname, namespace string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:     appName,
		k8sutil.ClusterAttr: namespace,
		"edgefs_svcname":    svcname,
		"edgefs_svctype":    "iscsi",
	}
}

// Validate the ISCSI arguments
func validateService(context *clusterd.Context, s edgefsv1alpha1.ISCSI) error {
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

// Check if the ISCSI service exists
func serviceExists(context *clusterd.Context, s edgefsv1alpha1.ISCSI) (bool, error) {
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

func getTargetParamsJSON(params edgefsv1alpha1.TargetParametersSpec) string {
	result := make(map[string]uint)

	if params.MaxRecvDataSegmentLength > 0 {
		result["MaxRecvDataSegmentLength"] = params.MaxRecvDataSegmentLength
	}

	if params.DefaultTime2Retain > 0 {
		result["DefaultTime2Retain"] = params.DefaultTime2Retain
	}

	if params.DefaultTime2Wait > 0 {
		result["DefaultTime2Wait"] = params.DefaultTime2Wait
	}

	if params.FirstBurstLength > 0 {
		result["FirstBurstLength"] = params.FirstBurstLength
	}

	if params.MaxBurstLength > 0 {
		result["MaxBurstLength"] = params.MaxBurstLength
	}

	if params.MaxQueueCmd > 0 {
		result["MaxQueueCmd"] = params.MaxQueueCmd
	}

	jsonString, _ := json.Marshal(result)
	return string(jsonString)
}
