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

package target

import (
	edgefsv1alpha1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1beta1 "k8s.io/api/apps/v1beta1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

const (
	dataDirsEnvVarName = "ROOK_DATA_DIRECTORIES"

	udpTotemPortDefault = int32(5405)
	udpTotemPortName    = "totem"
	volumeNameDataDir   = "datadir"

	/* Volumes definitions */
	configVolumeName  = "edgefs-configdir"
	configName        = "edgefs-config"
	dataVolumeName    = "edgefs-datadir"
	stateVolumeFolder = ".state"
	etcVolumeFolder   = ".etc"
)

func (c *Cluster) createAppLabels() map[string]string {
	return map[string]string{
		k8sutil.AppAttr: appName,
	}
}

func (c *Cluster) makeCorosyncContainer(containerImage string) v1.Container {

	privileged := c.deploymentConfig.needPriviliges
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		Privileged:             &privileged,
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		Capabilities: &v1.Capabilities{
			Add: []v1.Capability{"SYS_NICE", "IPC_LOCK"},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/etc",
			SubPath:   etcVolumeFolder,
		},
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/var/run",
			SubPath:   stateVolumeFolder,
		},
		{
			Name:      configVolumeName,
			MountPath: "/opt/nedge/etc/config",
		},
		{
			Name:      dataVolumeName,
			MountPath: "/data",
		},
	}

	return v1.Container{
		Name:            "corosync",
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{"corosync"},
		SecurityContext: securityContext,
		//Resources:       c.resources,
		VolumeMounts: volumeMounts,
		Env: []v1.EnvVar{
			{
				Name: "HOST_HOSTNAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
		},
	}
}

func (c *Cluster) makeAuditdContainer(containerImage string) v1.Container {

	privileged := c.deploymentConfig.needPriviliges
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		Privileged:             &privileged,
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/etc",
			SubPath:   etcVolumeFolder,
		},
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/var/run",
			SubPath:   stateVolumeFolder,
		},
	}

	return v1.Container{
		Name:            "auditd",
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            []string{"auditd"},
		Env: []v1.EnvVar{
			{
				Name:  "CCOW_LOG_LEVEL",
				Value: "5",
			},
			{
				Name: "HOST_HOSTNAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
		},
		SecurityContext: securityContext,
		//Resources:       c.resources,
		VolumeMounts: volumeMounts,
	}
}

func (c *Cluster) makeDaemonContainer(containerImage string, dro DevicesResurrectOptions, isInitContainer bool) v1.Container {

	privileged := c.deploymentConfig.needPriviliges
	runAsUser := int64(0)
	readOnlyRootFilesystem := false
	securityContext := &v1.SecurityContext{
		Privileged:             &privileged,
		RunAsUser:              &runAsUser,
		ReadOnlyRootFilesystem: &readOnlyRootFilesystem,
		Capabilities: &v1.Capabilities{
			Add: []v1.Capability{"SYS_NICE", "SYS_RESOURCE"},
		},
	}

	volumeMounts := []v1.VolumeMount{
		{
			Name:      "devices",
			MountPath: "/dev",
		},
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/etc",
			SubPath:   etcVolumeFolder,
		},
		{
			Name:      dataVolumeName,
			MountPath: "/opt/nedge/var/run",
			SubPath:   stateVolumeFolder,
		},
	}

	if c.deploymentConfig.deploymentType == deploymentAutoRtlfs {
		volumeMounts = append(volumeMounts, v1.VolumeMount{Name: dataVolumeName, MountPath: "/data"})
	} else if c.deploymentConfig.deploymentType == deploymentRtlfs {
		rtlfsDevices := getRtlfsDevices(c.Storage.Directories)
		for _, device := range rtlfsDevices {
			volumeMounts = append(volumeMounts, v1.VolumeMount{Name: device.Name, MountPath: device.Path})
		}
	}

	name := "daemon"
	args := []string{"daemon"}
	if isInitContainer {
		if dro.needToZap {
			name = "daemon-zap"
			args = []string{"toolbox", "nezap --do-as-i-say"}

			// zap mode is InitContainer, it needs to mount config
			volumeMounts = append(volumeMounts, v1.VolumeMount{
				Name:      configVolumeName,
				MountPath: "/opt/nedge/etc/config",
			})
		}
	} else {
		if dro.needToWait {
			args = []string{"wait"}
		}
	}

	return v1.Container{
		Name:            name,
		Image:           containerImage,
		ImagePullPolicy: v1.PullAlways,
		Args:            args,
		Env: []v1.EnvVar{
			{
				Name:  "CCOW_LOG_LEVEL",
				Value: "5",
			},
			{
				Name: "HOST_HOSTNAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
		},
		SecurityContext: securityContext,
		Resources:       c.resources,
		VolumeMounts:    volumeMounts,
	}
}

func (c *Cluster) configOverrideVolume() v1.Volume {
	cmSource := &v1.ConfigMapVolumeSource{Items: []v1.KeyToPath{{Key: "nesetup", Path: "nesetup.json"}}}
	cmSource.Name = configName
	return v1.Volume{Name: configVolumeName, VolumeSource: v1.VolumeSource{ConfigMap: cmSource}}
}

func isHostNetworkDefined(hostNetworkSpec edgefsv1alpha1.NetworkSpec) bool {
	if len(hostNetworkSpec.ServerIfName) > 0 || len(hostNetworkSpec.ServerIfName) > 0 {
		return true
	}
	return false
}

func (c *Cluster) createPodSpec(rookImage string, dro DevicesResurrectOptions) v1.PodSpec {
	terminationGracePeriodSeconds := int64(60)

	DNSPolicy := v1.DNSClusterFirst
	if isHostNetworkDefined(c.HostNetworkSpec) {
		DNSPolicy = v1.DNSClusterFirstWithHostNet
	}

	volumes := []v1.Volume{
		{
			Name: "devices",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/dev",
				},
			},
		},
	}

	hostPathDirectoryOrCreate := v1.HostPathDirectoryOrCreate
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
		volumes = append(volumes, v1.Volume{
			Name: dataVolumeName,
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: c.dataDirHostPath,
					Type: &hostPathDirectoryOrCreate,
				},
			},
		})
	}

	if c.deploymentConfig.deploymentType == deploymentRtlfs {
		// RTLFS with specified folders
		for _, folder := range c.deploymentConfig.directories {
			volumes = append(volumes, v1.Volume{
				Name: folder.Name,
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: folder.Path,
						Type: &hostPathDirectoryOrCreate,
					},
				},
			})

		}

	}

	var containers []v1.Container
	var initContainers []v1.Container
	if dro.needToZap {
		// To execute "zap" functions (devices or directories) we
		// create InitContainer to ensure it completes fully.
		initContainers = []v1.Container{
			c.makeDaemonContainer(rookImage, dro, true),
		}
	}

	volumes = append(volumes, c.configOverrideVolume())

	containers = []v1.Container{
		c.makeCorosyncContainer(rookImage),
		c.makeAuditdContainer(rookImage),
		c.makeDaemonContainer(rookImage, dro, false),
	}

	return v1.PodSpec{
		ServiceAccountName: c.serviceAccount,
		Affinity: &v1.Affinity{
			PodAntiAffinity: &v1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
					{
						Weight: int32(100),
						PodAffinityTerm: v1.PodAffinityTerm{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      k8sutil.AppAttr,
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{appName},
									},
								},
							},
							TopologyKey: apis.LabelHostname,
						},
					},
				},
			},
		},
		NodeSelector:   map[string]string{c.Namespace: "cluster"},
		InitContainers: initContainers,
		Containers:     containers,
		//  No pre-stop hook is required, a SIGTERM plus some time is all that's needed for graceful shutdown of a node.
		TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
		DNSPolicy:                     DNSPolicy,
		HostIPC:                       true,
		HostNetwork:                   isHostNetworkDefined(c.HostNetworkSpec),
		Volumes:                       volumes,
	}
}

func (c *Cluster) makeStatefulSet(replicas int32, rookImage string, dro DevicesResurrectOptions) (*appsv1beta1.StatefulSet, error) {

	statefulSet := &appsv1beta1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: c.Namespace,
			Labels:    c.createAppLabels(),
		},
		Spec: appsv1beta1.StatefulSetSpec{
			ServiceName: appName,
			Replicas:    &replicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: c.Namespace,
					Labels:    c.createAppLabels(),
				},
				Spec: c.createPodSpec(rookImage, dro),
			},
			PodManagementPolicy: appsv1beta1.ParallelPodManagement,
			UpdateStrategy: appsv1beta1.StatefulSetUpdateStrategy{
				Type: appsv1beta1.RollingUpdateStatefulSetStrategyType,
			},
		},
	}

	if c.dataVolumeSize.Value() > 0 {
		statefulSet.Spec.VolumeClaimTemplates = []v1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dataVolumeName,
					Namespace: c.Namespace,
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceStorage: c.dataVolumeSize,
						},
					},
				},
			},
		}
	}

	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &statefulSet.ObjectMeta, &c.ownerRef)
	c.placement.ApplyToPodSpec(&statefulSet.Spec.Template.Spec)

	return statefulSet, nil
}

func (c *Cluster) makeHeadlessServicePorts(totemPort int32) []v1.ServicePort {
	return []v1.ServicePort{
		{
			// The secondary port serves the UI as well as health and debug endpoints.
			Name:       udpTotemPortName,
			Port:       int32(totemPort),
			Protocol:   v1.ProtocolUDP,
			TargetPort: intstr.FromInt(int(totemPort)),
		},
	}
}

// This service only exists to create DNS entries for each pod in the stateful
// set such that they can resolve each other's IP addresses. It does not
// create a load-balanced ClusterIP and should not be used directly by clients
// in most circumstances.
func (c *Cluster) makeHeadlessService() (*v1.Service, error) {

	headlessService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: c.Namespace,
			Labels:    c.createAppLabels(),
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
		},
		Spec: v1.ServiceSpec{
			Selector:                 c.createAppLabels(),
			PublishNotReadyAddresses: true,
			ClusterIP:                "None",
			Ports:                    c.makeHeadlessServicePorts(udpTotemPortDefault),
		},
	}
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &headlessService.ObjectMeta, &c.ownerRef)

	return headlessService, nil
}
